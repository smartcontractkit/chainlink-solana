# Data Feeds Cache

## Reference
- **[Forwarder](../forwarder/README.md)** 

## Introduction

The data feeds cache (DFC) is a receiver program that is the receipient of the forwarder contract's payload. For more information on how this works, please reference the forwarder documentation. 

In general, the Solana DFC has the same features as a [prior version](https://github.com/smartcontractkit/chainlink-data-feeds-2.0/blob/develop/contracts/src/v0.8/data-feeds/DataFeedsCache.sol) (as opposed to the [latest version](https://github.com/smartcontractkit/chainlink/blob/83ccf038841caaaf97f404d71c585bdd3232cc22/contracts/src/v0.8/data-feeds/DataFeedsCache.sol)). You should use the older version as reference (however some of the behavior such as emitting events for invalid report permission and stale reports instead of reverting is similar to the latest EVM version):
1. Configure the decimal reports
    * updates permission storage variable(s)
    * updates feed config storage variables(s)
2. Accept payload from the forwarder in `on_report` and update the feed report

However there are a couple differences in the Solana DFC
1. Does not support proxying, so there is no such mapping to maintain a proxy to a dataIds. In this way, it's more similar to the older version of EVM DFC which does not support proxying.
2. Before updating the decimal report config of a feed, you must simulate `preview_decimal_feed_config` instruction in order to know ahead of time what defunct write permission accounts to remove. 
3. Support for writing to Solana DF 1.0 legacy store and feeds. This can be enabled or disabled depending on flags and optional accounts passed in.
4. We only support decimal reports and not bundled reports (product decision)

In order to accomodate variable number of feeds being configured and reported to at a time, the Solana DFC relies greatly on implicit accounts in the context (ctx.remaining_accounts).

## Important Accounts 

To best introduce the core mechanisms of this program, let's look at the essential accounts required to report a decimal report feed.

### 1. The decimal report feed account

Where the price/value is actually stored 

```
#[account]
#[derive(InitSpace)]
pub struct DecimalReport {
    pub timestamp: u32,
    pub answer: u128,
}
```

It's a PDA with the following derivation

Where cache_state refers to the specific cache instance and data id is the unique identifier for a data feed.

```
#[account(
     init,
     seeds = [
         b"decimal_report",
         cache_state.key().as_ref()
         data_id,
     ],
     bump
 )]
pub report: UncheckedAccount<'info>
```

Decimal reports are created via the `system_instruction::create_account` in the `init_decimal_reports` instruction in order to create multiple of them at the same time.

### 2. The decimal feed config account

This is a zero copy account. The workflow_metadata is a fixed size arrayvec which stores, similar to the EVM version, the associated workflows (sender, owner, name) that are authorized to report on the data feed. Note that because workflow_metadata is fixed, there is a hard cap on how many workflows can be assigned to a single feed.

```
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct FeedConfig {
    // UTF-bytes encoded
    pub description: [u8; 32],
    pub workflow_metadata: WorkflowMetadataList
}
```

```
#[account(
    mut,
    seeds = [
        b"feed_config",
        state.key().as_ref()
        data_id,
    ],
    bump
)]
pub feed_config: UncheckedAccount<'info>
```

### 3. The write permission flag account

Like in the EVM implementation, this write flag determines whether or not a workflow is authorized to report on behalf of a certain data feed.

If a feed has 3 workflows assigned to it, that means there will be 3 write permission flag accounts. 

The write permission flag is an empty struct.

```
#[account]
#[derive(Default)]
pub struct WritePermissionFlag {}

```

```
   #[account(
        mut,
        seeds = [
            b"permission_flag",
            state.key().as_ref()
            report_hash,
        ],
        bump
    )]
    pub permission_flag: UncheckedAccount<'info>

```

Note: In the `set_decimal_feed_configs`, because we are configuring N feeds with M workflow metadatas, this means there will be NxM write permission accounts that will need to be referenced (each of the N feeds has M permission flag accounts)

### 4. Cache State Account

This account represents a new DFC instance. It keeps track of ownership, admins, and the legacy writer PDA associated with the instance. 


```
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct CacheState {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub feed_admins: AdminList,
    pub legacy_writer_bump: u8, // pda writing to the legacy feeds
    pub _padding: [u8; 7],
}
```

* The feed admins list is also a fixed size arrayvec.
* The legacy writer bump is the bump of the PDA that is responsible for signing the `invoke_signed` CPI to the legacy store for writing legacy feeds.
* The padding is for byte alignment.


### 5. Legacy Feeds Config

This is the config account that tells us what data ids are associated with DF 1.0 legacy feeds and the legacy store account which is invoked by this program (if enabled)

It is also a zero copy account
```
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct LegacyFeedsConfig {
    pub id_to_feed: LegacyFeedList,
    pub legacy_store: Pubkey,
}
```

LegayFeedList is another...you guessed it.. fixed sized arrayvec. It is sorted in ascending order by data id for quick lookup to check whether a data id has an associated legacy feed. 

Each entry in the arrayvec is a LegacyFeedEntry

```
#[zero_copy]
#[derive(InitSpace)]
pub struct LegacyFeedEntry {
    pub data_id: [u8; 16],
    pub legacy_feed: Pubkey,
    // functions mainly as a killswitch in case of emergencies
    // under normal operations, this is expected to be 0
    // 0 = enabled. 1 = disabled
    // regardless of what this flag is, if legacy_store or legacy_feed_config is not passed into report, writes cannot occur
    pub write_disabled: u8,
}
```

where write_disabled is a flag that can be turned on in case of emergencies in order to decouple the write DFC with the legacy store contract quickly.

```
  #[account(
        init,
        payer = owner,
        space = ANCHOR_DISCRIMINATOR + LegacyFeedsConfig::INIT_SPACE, // todo add legacy feeds config size
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,

```









1. setting the decimal feed config 

For the two optional accounts in the OnReport context you have to pass in the cache program id in order for the accounts to be None.

## Notes

### initialize

The cache state account is a keypair account that's ownership is transferred to the program. 

We pre-compute the legacy writer bump to store in the cache state for usage later in invoke_signed (in `on_report`)

### init_decimal_reports

Initializes the decimal reports so that it can be written to in the `on_report` instruction.

The N data report accounts passed into the ctx.remaining_accounts must match
the order that the data ids are passed in or else the instruction will revert.

```
pub fn init_decimal_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, InitDecimalReports<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
```

```
#[derive(Accounts)]
pub struct InitDecimalReports<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    pub system_program: Program<'info, System>,
    // N data report accounts
    // #[account(
    //     init,
    //     seeds = [
    //         b"decimal_report",
    //         cache_state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    // )]
    // pub report: UncheckedAccount<'info>
}
```

### init_legacy_feeds_config 

Contains legacy store / feed information. As a recap, the DF 1.0 store owns the feed accounts which contain the actual data. There is 1 store program and multiple feed accounts which are updated.

For flexibility, the legacy_store is not hardcoded to be the the Program<Store> in order to provide flexibility and compatibility with any contract which implements the expected `cache_submit` function.

```
    pub fn init_legacy_feeds_config(
        ctx: Context<InitLegacyFeedsConfig>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<> {}
```

The data ids must be sorted in ascending order.

The data ids passed into the instruction will be paired with the legacy feed accounts passed into ctx.remaining_accounts, so order of the ctx.remaining_accounts is important.

By default, all legacy feed entries will have write disabled set to 0, so in other words writes are enabled. However, as you will see later, this does not mean that the legacy feeds will be written to necessarily. Basically, write_disabled = 0 means it MAY write to legacy feeds (if the legacy feed store and config are passed in on_report) but write_disabled = 1 means that is will definitely NOT write to the legacy feed.

```
#[derive(Accounts)]
pub struct InitLegacyFeedsConfig<'info> {
    #[account(mut, address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    #[account(executable)]
    /// CHECK: We don't use Program<> here since it can be any program that obeys the interface, "executable" is enough
    pub legacy_store: UncheckedAccount<'info>,

    #[account(
        init,
        payer = owner,
        space = ANCHOR_DISCRIMINATOR + LegacyFeedsConfig::INIT_SPACE, // todo add legacy feeds config size
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,

    pub system_program: Program<'info, System>,

    // in ctx.remaining_accounts N legacy feeds (to match N legacy data ids)
    // we do not enforce an account type because the account struct is subject to change
    // and knowing its schema is not the responsibility of the cache program but the store
    // we just need to know what the account address is for verification purposes
    // pub legacy_feed: UncheckedAccount<'info>
}
```

### update_legacy_feeds_config

Similar to init_legacy_feeds_config however it enables you to disable legacy feed writes. 

### set_decimal_feed_configs

Because this function takes in a variable number of data ids and workflows, the transaction limit
may be met if those bounds are exceeded. 

Below is a table of acceptable ranges where `N` (row) is the number of data ids and `M` (column) is the number of workflows. The table value is the estimated number of bytes. This is meant to be a guideline and does 
not necessarily guarentee the success of the transaction.

Note, the table below assumes 0 deleted write permission flag accounts. Please account for them by adding to the estimated transaction size.

| (N, M) | 1   | 2   | 3     | 4    | 5    | 6    | 7    | 8    | 9 | 10   |
|-----|-----|-----|-------|------|------|------|------|------|------|------|
| 1   | 302 | 396 | 490   | 584  | 678  | 772  | 866  | 960  | 1054 | 1148 | 
| 2   | 414 | 540 | 666   | 792  | 918  | 1044 |      |      |      |      |      
| 3   | 526 | 684 | 842   | 1000 |      |      |      |      |      |      |      
| 4   | 638 | 828 | 1018  | 1208 |      |      |      |      |      |      |      
| 5   | 750 | 972 | 1194  |      |      |      |      |      |      |      |      
| 6   | 862 | 1116|       |      |      |      |      |      |      |      |      
| 7   | 974 |     |       |      |      |      |      |      |      |      |      
| 8   | 1086|     |       |      |      |      |      |      |      |      |      
| 9   | 1198|     |       |      |      |      |      |      |      |      |         

Given N data ids, N descriptions, and M workflows, this instruction will
1. create (if doesn't exist) and update the N decimal feed config accounts
2. create (if doesn't exist) N x M permission flag accounts (M for each data id)
3. mark stale permission accounts (for defunct workflows) if any exist

note: you will be prevented from setting the decimal feed configs if it realizes you haven't closed the stale permission accounts from a prior operation.

```
 pub fn set_decimal_feed_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, SetDecimalFeedConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<[u8; 32]>,
        workflow_metadatas: Vec<WorkflowMetadata>,
    ) -> Result<()> {}
```

Passing in the remaining_accounts is as follows:

First, you have N feed config accounts which match the order of the `data_ids` parameter passed into the instruction

Then, you have N x M permission flag accounts which are ordered by the data_id first and workflow second. 

Lastly you have L defunct permission accounts which are to be closed. You get these by simulating the preview_decimal_feed_configs instruction and passing them here.

Below is an example.

```
// ex: data_ids: [1, 2]
// workflow metadatas [5, 6, 7]
// ctx remaining accounts:
// [1-feed-config]  |- feed_config_accounts
// [2-feed-config]  |
// [flag-1-5] [flag-1-6] [flag-1-7]  |- permission_flag_accounts
// [flag-2-5] [flag-2-6] [flag-2-7]  |
```

```
#[derive(Accounts)]
pub struct SetDecimalFeedConfigs<'info> {
    // todo: inline check if it is an admin
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    pub system_program: Program<'info, System>,
    // dynamic list of writePermissions. create if not created already, or overwrite as well

    // N accounts, N = # of data ids
    //   #[account(
    //     mut,
    //     seeds = [
    //         b"feed_config",
    //         state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    //   )]
    //   pub feed_config: UncheckedAccount<'info>

    // N X M accounts, N = # of data_ids, M = # of workflows
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag",
    //         state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>


    // Defunct permission accounts that need closing
    // acquired by simulating "preview_decimal_feed_configs"
    // L accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag",
    //         state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>
}
```


### preview_decimal_feed_configs

This instruction is only meant to be simulated off-chain before calling `set_decimal_feeds_config`. It does not change account state. This is to preview the permission accounts that are not referenced anymore and thus can be deleted. For example, let's say originally your workflow_metadata is [A, B, C]. If you update feedConfig to [A, B, D], C is no longer authorized to report on behalf of the feed, so it's permission account is closed in `set_decimal_feed_configs`. You must however know this ahead of time because you are required to specify all accounts that are touched.

Note that the account context is different between set_decimal_feed_configs and preview_decimal_feed_configs. Anyone can call this instruction and of course you don't pass in the defunct account permissions in this context (figuring that out is the purpose of this instruction)

```
#[derive(Accounts)]
pub struct PreviewDecimalFeedConfigs<'info> {
    pub state: AccountLoader<'info, CacheState>,
    // dynamic list of writePermissions. create if not created already, or overwrite as well

    // N accounts, N = # of data ids
    //   #[account(
    //     mut,
    //     seeds = [
    //         b"feed_config",
    //         state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    //   )]
    //   pub feed_config: UncheckedAccount<'info>

    // N X M accounts, N = # of data_ids, M = # of workflows
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag",
    //         state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>
}
```

### on_report

The hook that the forwarder calls in order to deliver a batched decimals report update.

It follows the expected instruction signature that all forwarders require.

```
pub fn on_report<'info>(
        ctx: Context<'_, '_, '_, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {}
```

And additionally, the first two accounts in the account context are the forwarder_state and forwarder_authority (as expected)

```
#[derive(Accounts)]
pub struct OnReport<'info> {
    // #[account(owner = FORWARDER_ID)]
    // checking the owner of the state is optional and not necessary
    // because the forwarder state is uniquely associated with the
    // forwarder authority which is verified in the instruction
    // warning: the FORWARDER_ID deployed in an environment may be different
    // than the one in source control. you need to view the docs to determine
    // what the actual deployed program id is.
    pub forwarder_state: Account<'info, ForwarderState>,

    #[account(seeds = [b"forwarder", forwarder_state.key().as_ref(), <RECEIVER_PROGRAM_ID>], bump = forwarder_state.authority_nonce, seeds::program = FORWARDER_ID)]
    pub forwarder_authority: Signer<'info>,

    #[account()]
    pub cache_state: AccountLoader<'info, CacheState>,

    // some data cache instances may not care about the legacy feeds so they
    // will omit them both and legacy feed logic will be skipped

    // omit if you don't want to write to the store
    #[account(executable)]
    pub legacy_store: Option<UncheckedAccount<'info>>,

    // omit if you don't want to write to the store
    #[account(
        seeds = [b"legacy_feeds_config", cache_state.key().as_ref()], // todo: add the current state
        bump
    )]
    pub legacy_feeds_config: Option<AccountLoader<'info, LegacyFeedsConfig>>,

    // omit if you don't want to write to the store
    /// CHECK: This is a PDA
    #[account(seeds = [b"legacy_writer", cache_state.key().as_ref()], bump = cache_state.load()?.legacy_writer_bump)]
    pub legacy_writer: Option<UncheckedAccount<'info>>,

    pub system_program: Program<'info, System>,
    // remaining accounts (N data ids, M legacy feeds)

    // N accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"decimal_report",
    //         cache_state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    // )]
    // pub report: UncheckedAccount<'info>

    // N accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag",
    //         cache_state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>

    // M transmission feed accounts
    // should be sorted
    //
    // included if and only if both legacy_store and legacy_feeds_config is included.
    // if only 1 or 0 or the legacy_store / legacy_feeds_config accounts are included
    // this should not be included.
    //
    // note: not all of the legacy feed accounts supplied may be written to because there is
    // a write_disabled flag per account. assume this is sorted.
    //
    // pub legacy_feed: UncheckedAccount<'info>
}
```

There are three optional accounts, all related to legacy feed writing. If you omit any of these (by passing in the DFC program id) or have write_disabled = 1 for all feeds, then we guarentee no legacy feeds will be written to.

These are deemed optional mainly for saving space if we deploy programs (because we may deploy many DFC programs depending on the circumstances) that don't require their usage at all or for the future when the legacy feeds are no longer supported (all customers have migrated to the DFC) and we can save space by removing the legacy-feed related accounts entirely.

The structure of the remaining accounts is
*  N decimal report accounts (following order of the data ids of the received reports payload)
* Then the N permission flags are listed (again following order of the order of the data ids in the received reports payload)
* Then at the very end are all the legacy feed accounts that are associated with the received reports in the payload. For example, if the received reports is reporting on feeds that do not have a legacy feed this will be 0, otherwise it will be non-zero. 

Note that if you supply legacy_feeds in remaining accounts that are not required nothing bad will occur. If you have disabled the writes through any of the mechanism listed earlier, you can also omit passing legacy feeds in the ctx.remaining_accounts entirely. However, if you legacy feeds are enabled, then you must supply all the legacy feed accounts in ctx.remaining_accounts or else the transaction will revert because it expects it to included.

So for internal data feeds use case we said in the forwarder README that
```
 max_payload_size = 297
```

Based on the payload encoding and account contexts of data feed cache on_report, here are the estimated number of decimal reports that can be sent in one transmission:


Best case (no legacy feeds)
```
297 = 4 + 40*N + (cache_state (1) + 2*N)

N = 6.9
```
* 4 + 40*N is the total payload size for N `ReceivedDecimalReport`s
* Remember we're using the address lookup table, so accounts take 1 byte only

Worst case (all reports are tied with legacy feeds)


```
297 = 4 + 40*N + (cache_state (1) + legacy_store (1) + legacy_feed_config (1) + legacy_writer (1)  + 3N)

N = 6.6
```
* in the account context calculations, we use 3N over 2N because the extra N comes from the legacy feed accounts in ctx.remaining_accounts

Rounding down, we can at most support 6 decimal feed reports with ALTs



