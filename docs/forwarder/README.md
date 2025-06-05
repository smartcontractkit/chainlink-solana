# Forwarder

## Overview

The forwarder verifies that the a report comes from a valid oracle network and routes it to a receiver program.

Let's look at the accounts associated with a report instruction to understand what it does.

```
#[derive(Accounts)]
#[instruction(data: Vec<u8>)]
pub struct Report<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        mut,
        seeds = [b"config", state.key().as_ref(), &extract_config_id(extract_raw_report(&data))],
        bump
    )]
    oracles_config: Account<'info, OraclesConfig>,

    #[account(mut)]
    pub transmitter: Signer<'info>,

    /// CHECK: This is a PDA
    #[account(seeds = [b"forwarder", state.key().as_ref()], bump = state.authority_nonce)]
    pub forwarder_authority: UncheckedAccount<'info>,

    // it is dependent on the state.key(), a predetermined bump, workflow execution id, config_id, report_id
    #[account(
        init_if_needed,
        payer = transmitter,
        space = ANCHOR_DISCRIMINATOR + ExecutionState::INIT_SPACE,
        seeds = [
            b"execution_state", 
            state.key().as_ref(),
            &extract_transmission_id(extract_raw_report(&data), receiver_program.key)
        ],
        bump
    )]
    pub execution_state: Account<'info, ExecutionState>,

    #[account(executable)]
    /// CHECK: We don't use Program<> here since it can be any program, "executable" is enough
    pub receiver_program: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
    // remaining accounts passed to receiver
}
```

`state` is a keypair, program owned account which represents the forwarder instance we are working with. Think of this is a specific forwarder instance.

`oracles_config` is a PDA which stores DON info

`forwarder_authority` is the PDA which is responsible for signing the CPI to a receiver program via invoke_signed()

`execution_state` is the PDA which stores a report's execution status. For now, it only stores successes.

`receiver_program` the receiver of the CPI

`system_program` (self-explanatory)



The following instructions exist on the program:

- `initialize`: creates a new forwarder state account (like deploying a new forwarder in EVM)
- `transfer_ownership`: begin two-step ownership transfer
- `accept_ownership`: end two-step ownership transfer
- `init_oracles_config`: create PDA which stores information for a given don id and config version
- `update_oracles_config`: updates PDA
- `close_oracles_config`: closes PDA
- `report`: called by the Chainlink DON to pass a verified report along to a receiver program

## initialize

An important responsibility of this instruction is to pre-compute the forwarder authority PDA and store the bump in the state account for future usage. The bump (along with seed phrases) is used to deterministically compute the forwarder authority PDA. So this means that given a state account, known seeds, and the bump in authority nonce (or bump) you can deterministically derive the associated forwarder authority PDA.

## report

The report instruction verifies the report by checking it's ECDSA signatures and ensuring that f + 1 nodes have signed the report. After verification it will create a PDA to store the execution state if it does not exist. 

If everything looks good, it'll begin constructing the CPI instruction. The report function will pass through ctx.remaining_accounts to the receiver, but it assumes the forwarder authority is the sole signer.


The report function takes a raw data buffer `Vec<u8>` with a custom encoding format to save space

```
data =  len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
```

The `report_context` is extra information about the report that is included before signing the entire report blob.

The `report` instruction expects the receiver to implement an `on_report` function that looks like so:


// Receiver contract will implement this in Anchor (or equivalent in pure Rust)

```
pub fn on_report(ctx: Context<OnReport>, metadata: Vec<u8>, report: Vec<u8>) -> Result<()> {...}
```

```
// with the following declared accounts

#[derive(Accounts)]
pub struct OnReport<'info> {
    #[account(owner = FORWARDER_ID)]
    pub state: Account<'info, ForwarderState>,

    /// CHECK: This is a PDA
    /// Anchor is unable to compute PDA with other program id so must do inline check within on_report
    /// #[account(seeds = [b"forwarder", state.key().as_ref()], bump = state.authority_nonce)]
    pub forwarder_authority: Signer<'info>,

    // remaining accounts passed in as well
}
```

As noted above, it is crucial that the receiver program verify the forwarder authority is a PDA derived from the forwarder state account.

## transaction size

Solana places a hard 1232 byte limit on transaction sizes. For the report instruction, we want to make sure we have enough space left over for the receiver payload given that accounts and signatures will take up significant space in the transaction.

The anchor tests simulate the transaction so we can get a good idea of the amount leftover. To make things simple, the test has a 1 byte payload and only passes 1 additional remaining account in the report instruction.

We can use ALTs (address lookup tables) to free up more transaction space. ALTs are enabled in V0 transactions. They allow a user to store account addresses on-chain, and pass in the ALT account and a list of byte indexes to refer to a program. So instead of passing 32 bytes for each account, you'd pass in a 1 byte index on top of specifying one or more ALT addresses. 

We use ALTs (address lookup tables) for the following accounts for a hypothetical data feeds use case (see test usage [here](../../contracts/tests/keystone_forwarder.spec.ts#L430) and result [here](../../contracts/tests/keystone_forwarder.spec.ts#L482)):
* forwarder state
* oracles config pda,
* forwarder authority pda, 
* receiver program
* system program
* receiver data account state which stores some arbitrary data (part of ctx.remaining_accounts)

This uses 901 bytes, so we have 1232 - 901 = 331 bytes left over for the payload. 

This 331 number accounts for a test 1 byte payload and also an extra single data account used by the receiver, so it'd be 333 bytes with a clean slate.

In an internal data feeds use case we can assume that the receiver program is static and that the data accounts we are writing to per price asset are also static. However, a realistic use case would definitely have extra data accounts passed into ctx.remaining_accounts. 

So for internal data feeds use case:
```
max_payload_size = 333 - (ctx.remaining_accounts.len())
```

For an external use-case, for an arbitrary receiver, the user will need to pass in another ALT (Solana can support passing 4 ALTs per transaction) on top of the internal ALT we always pass in. So we lose 32 bytes. Assuming the user also puts the receiver program and extra data accounts in their personal ALT:
```
max_payload_size = 333 - 32 = 301 - (ctx.remaining_accounts.len())
```

If they don't pass in another ALT:

```
max_payload_size = 333 - 32 - 32*(ctx.remaining_accounts.len()) = 301 - 32*(ctx.remaining_accounts.len()) 
```
(the first 32 bytes is deducted for the receiver account address)



