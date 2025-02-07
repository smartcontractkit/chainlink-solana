use anchor_lang::prelude::*;
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

    pub fn initialize_lookup_table(
        ctx: Context<InitializeLookupTableData>,
        lookup_table: Pubkey,
    ) -> Result<()> {
        let account = &mut ctx.accounts.write_data_account;
        account.version = 1;
        account.administrator = ctx.accounts.admin.key();
        account.pending_administrator = Pubkey::default();
        account.lookup_table = lookup_table;

        Ok(())
    }

    pub fn store(ctx: Context<StoreTestStruct>, test_idx: u64, data: TestStructData) -> Result<()> {
        let test_struct_account = &mut ctx.accounts.test_struct.load_init()?;

        test_struct_account.idx = test_idx;
        test_struct_account.bump = ctx.bumps.test_struct;

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
pub struct InitializeLookupTableData<'info> {
    /// PDA for LookupTableDataAccount, derived from seeds and created by the System Program
    #[account(
        init_if_needed,
        payer = admin,
        space = size_of::<LookupTableDataAccount>() + 8,
        seeds = [b"data"],
        bump
    )]
    pub write_data_account: Account<'info, LookupTableDataAccount>,

    /// Admin account that pays for PDA creation and signs the transaction
    #[account(mut)]
    pub admin: Signer<'info>,

    /// System Program required for PDA creation
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct StoreTestStruct<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    #[account(
        init_if_needed,
        payer = signer,
        space = size_of::<TestStruct>() + 8,
        seeds=[b"struct_data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump
    )]
    pub test_struct: AccountLoader<'info, TestStruct>,

    pub system_program: Program<'info, System>,
}

#[account]
pub struct LookupTableDataAccount {
    pub version: u8,                   // Version of the data account
    pub administrator: Pubkey,         // Administrator public key
    pub pending_administrator: Pubkey, // Pending administrator public key
    pub lookup_table: Pubkey,          // Address of the lookup table
}

#[account]
pub struct DataAccount {
    pub idx: u64,
    pub bump: u8,
    pub u64_value: u64,
    pub u64_slice: Vec<u64>,
}

#[account(zero_copy)]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct TestStruct {
    pub idx: u64,
    pub bump: u8,
    _padding0: [u8; 7],
    pub field: i32,
    _padding1: [u8; 4],
    pub oracle_id: u8,
    _padding2: [u8; 15],
    pub oracle_ids: [u8; 32],
    pub accounts: [[u8; 32]; 2],
    pub different_field: [u8; 32], // hiding field since string does not play well with zero copy
    _padding3: [u8; 8],
    pub big_field: i128,

    pub account_struct: AccountStruct,
    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}

#[zero_copy]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct TestStructData {
    pub field: i32,
    _padding0: [u8; 4],
    pub oracle_id: u8,
    _padding1: [u8; 15],
    pub oracle_ids: [u8; 32],
    pub accounts: [[u8; 32]; 2],
    pub different_field: [u8; 32],
    _padding2: [u8; 8],
    pub big_field: i128,

    pub account_struct: AccountStruct,
    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}

#[zero_copy]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct AccountStruct {
    pub account: Pubkey,
    pub account_str: Pubkey,
}

#[zero_copy]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct MidLevelDynamicTestStruct {
    pub fixed_bytes: [u8; 2],
    pub _padding: [u8; 6], // explicit padding to avoid uninitialized bytes for zero_copy
    pub inner: InnerDynamicTestStruct,
}

#[zero_copy]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct InnerDynamicTestStruct {
    pub i: i64,
    pub s: [u8; 32],
}

#[zero_copy]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct MidLevelStaticTestStruct {
    pub fixed_bytes: [u8; 2],
    pub _padding: [u8; 6], // explicit padding to avoid uninitialized bytes for zero_copy
    pub inner: InnerStaticTestStruct,
}

#[zero_copy]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct InnerStaticTestStruct {
    pub i: i64,
    pub a: Pubkey,
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
