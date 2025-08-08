use anchor_lang::prelude::*;
use solana_program::pubkey;
use std::mem::size_of;

declare_id!("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE");

#[program]
pub mod contract_reader_interface {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>, test_idx: u64, value: u64) -> Result<()> {
        let account = &mut ctx.accounts.data;
        account.u64_value = value;
        account.u64_slice = [3, 4].to_vec();
        account.idx = test_idx;
        account.bump = ctx.bumps.data;

        Ok(())
    }

    pub fn initializemultiread(ctx: Context<InitializeMultiReadOnce>) -> Result<()> {
        let multi_read1 = &mut ctx.accounts.multi_read1;
        multi_read1.a = 1;
        multi_read1.b = 2;
        multi_read1.c = true;

        let multi_read2 = &mut ctx.accounts.multi_read2;
        multi_read2.u = "Hello".to_string();
        multi_read2.v = true;
        multi_read2.w = [123, 456];

        Ok(())
    }

    pub fn initializemultireadwithparams(
        ctx: Context<InitializeMultiReadWithParamsOnce>,
    ) -> Result<()> {
        let multi_read3 = &mut ctx.accounts.multi_read3;
        multi_read3.a = 10;
        multi_read3.b = 20;
        multi_read3.c = true;

        let multi_read4 = &mut ctx.accounts.multi_read4;
        multi_read4.u = "olleH".to_string();
        multi_read4.v = true;
        multi_read4.w = [321, 654];

        Ok(())
    }

    pub fn initializetokenprices(
        ctx: Context<InitializeBillingTokenConfigWrapperOnce>,
    ) -> Result<()> {
        let config1 = &mut ctx.accounts.config_wrapper_account1;
        config1.config.usd_per_token = TimestampedPackedU224 {
            value: STATIC_VALUE1,
            timestamp: STATIC_TIMESTAMP1,
        };

        let config2 = &mut ctx.accounts.config_wrapper_account2;
        config2.config.usd_per_token = TimestampedPackedU224 {
            value: STATIC_VALUE2,
            timestamp: STATIC_TIMESTAMP2,
        };

        Ok(())
    }

    pub fn initializelookuptable(
        ctx: Context<InitializeLookupTableData>,
        lookup_table: Pubkey,
    ) -> Result<()> {
        let account = &mut ctx.accounts.write_data_account;
        account.version = 1;
        account.administrator = ctx.accounts.admin.key();
        account.pending_administrator = Pubkey::default();
        account.lookup_table = lookup_table;
        account.bump = ctx.bumps.write_data_account;

        Ok(())
    }

    pub fn storeval(ctx: Context<StoreVal>, test_idx: u64, value: u64) -> Result<()> {
        let data = &mut ctx.accounts.data;
        data.bump = ctx.bumps.data;
        data.idx = test_idx;
        data.u64_value = value;
        Ok(())
    }

    pub fn store(
        ctx: Context<StoreTestStruct>,
        test_idx: u64,
        _list_idx: u64,
        data: TestStructData,
    ) -> Result<()> {
        let test_struct_account = &mut ctx.accounts.test_struct;

        test_struct_account.idx = test_idx;
        test_struct_account.field = data.field;
        test_struct_account.oracle_id = data.oracle_id;
        test_struct_account.oracle_ids = data.oracle_ids;
        test_struct_account.accounts = data.accounts;
        test_struct_account.different_field = data.different_field;
        test_struct_account.big_field = data.big_field;
        test_struct_account.account_struct = data.account_struct;
        test_struct_account.nested_dynamic_struct = data.nested_dynamic_struct;
        test_struct_account.nested_static_struct = data.nested_static_struct;

        Ok(())
    }

    pub fn store_token_account(ctx: Context<StoreTokenAccount>, test_idx: u64) -> Result<()> {
        let account = ctx.accounts.token_account.to_account_info();
        require!(
            !account.data_is_empty(),
            TokenAccountError::UninitializedTokenAccount
        );

        let data = &mut ctx.accounts.data;
        data.idx = test_idx;
        data.account = account.key();
        data.bump = ctx.bumps.data;

        Ok(())
    }

    pub fn create_event_and_fail(_ctx: Context<Events>) -> Result<()> {
        emit!(StateChangedEvent {
            new_state: "Pending".to_string()
        });

        // Intentionally fail the transaction after emitting the event
        Err(ErrorCode::IntentionalFailure.into())
    }

    pub fn trigger_event(_ctx: Context<Events>, data: TestStructData) -> Result<()> {
        emit!(SomeEvent {
            field: data.field,
            oracle_id: data.oracle_id,
            big_field: data.big_field,
            oracle_ids: data.oracle_ids,
            accounts: data.accounts,
            different_field: data.different_field,
            account_struct: data.account_struct,
            nested_dynamic_struct: data.nested_dynamic_struct,
            nested_static_struct: data.nested_static_struct,
        });

        Ok(())
    }
}

#[derive(Accounts)]
pub struct Events<'info> {
    pub signer: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct Initialize<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    // derived test PDA
    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<DataAccount>() + 8,
        seeds=[b"data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, DataAccount>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct InitializeMultiReadOnce<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<MultiRead1>() + 8,
        seeds = [b"multi_read1"],
        bump)]
    pub multi_read1: Account<'info, MultiRead1>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<MultiRead2>() + 8,
        seeds = [b"multi_read2"],
        bump)]
    pub multi_read2: Account<'info, MultiRead2>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct InitializeMultiReadWithParamsOnce<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<MultiRead3>() + 8,
        seeds = [
            b"multi_read_with_params3",
            1u64.to_le_bytes().as_ref()
        ],
        bump)]
    pub multi_read3: Account<'info, MultiRead3>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<MultiRead4>() + 8,
        seeds = [
            b"multi_read_with_params4",
            1u64.to_le_bytes().as_ref()
        ],
        bump)]
    pub multi_read4: Account<'info, MultiRead4>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct InitializeBillingTokenConfigWrapperOnce<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<BillingTokenConfigWrapper>() + 8,
        seeds = [
            b"fee_billing_token_config",
            ADDRESS_1.as_ref()
        ],
        bump)]
    pub config_wrapper_account1: Account<'info, BillingTokenConfigWrapper>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<BillingTokenConfigWrapper>() + 8,
        seeds = [
            b"fee_billing_token_config",
            ADDRESS_2.as_ref()
        ],
        bump)]
    pub config_wrapper_account2: Account<'info, BillingTokenConfigWrapper>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct InitializeLookupTableData<'info> {
    /// Admin account that pays for PDA creation and signs the transaction
    #[account(mut)]
    pub admin: Signer<'info>,

    /// PDA for LookupTableDataAccount, derived from seeds and created by the System Program
    #[account(
        init_if_needed,
        payer = admin,
        space = size_of::<LookupTableDataAccount>() + 8,
        seeds = [b"lookup".as_ref()],
        bump
    )]
    pub write_data_account: Account<'info, LookupTableDataAccount>,

    /// System Program required for PDA creation
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(test_idx: u64, list_idx: u64)]
pub struct StoreTestStruct<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    #[account(
        init,
        payer = signer,
        space = 8 + size_of::<TestStruct>(),
        seeds = [
            b"struct_data".as_ref(),
            test_idx.to_le_bytes().as_ref(),
            list_idx.to_le_bytes().as_ref()
        ],
        bump,
    )]
    pub test_struct: Account<'info, TestStruct>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct StoreVal<'info> {
    /// Admin account that pays for PDA creation and signs the transaction
    #[account(mut)]
    pub admin: Signer<'info>,

    // derived test PDA
    #[account(
        mut,
        seeds=[b"data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, DataAccount>,

    /// System Program required for PDA creation
    pub system_program: Program<'info, System>,
}

#[account]
#[derive(Default)]
pub struct TestStruct {
    pub idx: u64,
    pub bump: u8,
    pub field: i32,
    pub oracle_id: u8,
    pub oracle_ids: [u8; 32],
    pub accounts: [[u8; 32]; 2],
    pub different_field: String,
    pub big_field: i128,

    pub account_struct: AccountStruct,
    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct TestStructData {
    pub field: i32,
    pub oracle_id: u8,
    pub oracle_ids: [u8; 32],
    pub accounts: [[u8; 32]; 2],
    pub different_field: String,
    pub big_field: i128,

    pub account_struct: AccountStruct,
    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Default)]
pub struct AccountStruct {
    pub account: Pubkey,
    pub account_str: Pubkey,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Default)]
pub struct MidLevelDynamicTestStruct {
    pub fixed_bytes: [u8; 2],
    pub inner: InnerDynamicTestStruct,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Default)]
pub struct InnerDynamicTestStruct {
    pub i: i64,
    pub s: String,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Default)]
pub struct MidLevelStaticTestStruct {
    pub fixed_bytes: [u8; 2],
    pub inner: InnerStaticTestStruct,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Default)]
pub struct InnerStaticTestStruct {
    pub i: i64,
    pub a: Pubkey,
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct StoreTokenAccount<'info> {
    /// Admin account that pays for PDA creation and signs the transaction
    #[account(mut)]
    pub admin: Signer<'info>,

    /// CHECK: test
    #[account(mut)]
    pub token_account: UncheckedAccount<'info>,

    // derived test PDA
    #[account(
        init_if_needed,
        payer = admin,
        space = size_of::<TokenAccountData>() + 8,
        seeds=[b"token_account".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, TokenAccountData>,

    /// System Program required for PDA creation
    pub system_program: Program<'info, System>,
}

#[account]
pub struct LookupTableDataAccount {
    pub version: u8,                   // Version of the data account
    pub administrator: Pubkey,         // Administrator public key
    pub pending_administrator: Pubkey, // Pending administrator public key
    pub lookup_table: Pubkey,          // Address of the lookup table
    pub bump: u8,
}

#[account]
pub struct DataAccount {
    pub idx: u64,
    pub bump: u8,
    pub u64_value: u64,
    pub u64_slice: Vec<u64>,
}

#[account]
pub struct TokenAccountData {
    pub idx: u64,
    pub bump: u8,
    pub account: Pubkey,
}

#[account]
pub struct MultiRead1 {
    pub a: u8,
    pub b: i16,
    pub c: bool,
}

#[account]
pub struct MultiRead2 {
    pub u: String,
    pub v: bool,
    pub w: [u64; 2],
}

#[account]
pub struct MultiRead3 {
    pub a: u8,
    pub b: i16,
    pub c: bool,
}

#[account]
pub struct MultiRead4 {
    pub u: String,
    pub v: bool,
    pub w: [u64; 2],
}

pub const ADDRESS_1: Pubkey = pubkey!("57FUKrjY7Dywph1bqNGztvtTGWcXvk5VLNCfAXtk6jqK");
pub const ADDRESS_2: Pubkey = pubkey!("47XyyAALxH7WeNT1DGWsPeA8veSVJaF8MHFMqBM5DkP6");

pub const STATIC_VALUE1: [u8; 28] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B,
];
pub const STATIC_TIMESTAMP1: i64 = 1_700_000_001;

pub const STATIC_VALUE2: [u8; 28] = [
    0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00,
    0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80, 0x90, 0xA0, 0xB0, 0xC0,
];
pub const STATIC_TIMESTAMP2: i64 = 1_800_000_002;

#[account]
#[derive(InitSpace, Debug)]
pub struct BillingTokenConfigWrapper {
    pub version: u8,
    pub config: BillingTokenConfig,
}

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct BillingTokenConfig {
    pub enabled: bool,
    pub mint: Pubkey,

    pub usd_per_token: TimestampedPackedU224,
    pub premium_multiplier_wei_per_eth: u64,
}

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct TimestampedPackedU224 {
    pub value: [u8; 28],
    pub timestamp: i64,
}

#[error_code]
pub enum TokenAccountError {
    #[msg("Uninitialized token account")]
    UninitializedTokenAccount,
}

#[error_code]
pub enum ErrorCode {
    #[msg("This error is intentionally triggered for testing purposes.")]
    IntentionalFailure,
}

#[event]
pub struct StateChangedEvent {
    pub new_state: String,
}

#[event]
pub struct SomeEvent {
    pub field: i32,
    pub oracle_id: u8,
    pub oracle_ids: [u8; 32],
    pub accounts: [[u8; 32]; 2],
    pub different_field: String,
    pub big_field: i128,

    pub account_struct: AccountStruct,
    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}
