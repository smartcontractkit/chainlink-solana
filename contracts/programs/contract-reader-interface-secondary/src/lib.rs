use anchor_lang::prelude::*;
use std::mem::size_of;

declare_id!("9SFyk8NmGYh5D612mJwUYhguCRY9cFgaS2vksrigepjf");

#[program]
pub mod contract_reader_interface_secondary {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>, test_idx: u64, value: u64) -> Result<()> {
        let account = &mut ctx.accounts.data;
        account.u64_value = value;
        account.idx = test_idx;
        account.bump = ctx.bumps.data;
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
        space = size_of::<Data>() + 8,
        seeds=[b"data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, Data>,

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
pub struct Data {
    pub u64_value: u64,
    pub idx: u64,
    pub bump: u8,
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
