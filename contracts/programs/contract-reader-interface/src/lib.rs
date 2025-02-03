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

        ctx.accounts.value.u64_value = 0;

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

    pub fn store_val(ctx: Context<StoreVal>, value: u64) -> Result<()> {
        let val = &mut ctx.accounts.value;
        val.u64_value = value;
        
        Ok(())
    }

    pub fn store_test_struct(
            ctx: Context<StoreTestStruct>,
            test_idx: u64,
            data: TestStructData,
        ) -> Result<()> {
            let test_struct_account = &mut ctx.accounts.test_struct;
    
            test_struct_account.idx = test_idx;
            test_struct_account.bump = ctx.bumps.test_struct;
    
            test_struct_account.field = data.field;
            test_struct_account.oracle_id = data.oracle_id;
            test_struct_account.oracle_ids = data.oracle_ids;
            test_struct_account.account_struct = data.account_struct;
            test_struct_account.accounts = data.accounts;
            test_struct_account.different_field = data.different_field;
            test_struct_account.big_field = data.big_field;
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
        init,
        payer = signer,
        space = size_of::<DataAccount>() + 8,
        seeds=[b"data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, DataAccount>,

    // derived test PDA
    #[account(
        init,
        payer = signer,
        space = size_of::<Value>() + 8,
        seeds=[b"val"],
        bump)]
    pub value: Account<'info, Value>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct InitializeLookupTableData<'info> {
    /// PDA for LookupTableDataAccount, derived from seeds and created by the System Program
    #[account(
        init,
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
pub struct StoreVal<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    // derived test PDA
    #[account(
        mut,
        seeds=[b"val"],
        bump)]
    pub value: Account<'info, Value>,
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct StoreTestStruct<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

   #[account(
        init,
        payer = signer,
        // Add extra buffer for variable fields
        space = 8 + size_of::<TestStruct>() + 400,
        seeds = [
            b"test-struct",
            test_idx.to_le_bytes().as_ref()
        ],
        bump
    )]
    pub test_struct: Account<'info, TestStruct>,

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

#[account]
pub struct Value {
    pub u64_value: u64
}

#[account]
pub struct TestStruct {
    pub idx: u64,
    pub bump: u8,

    pub field: Option<i32>,
    pub oracle_id: [u8; 32],
    pub oracle_ids: [[u8; 32]; 32],
    pub account_struct: AccountStruct,
    pub accounts: Vec<Vec<u8>>,
    pub different_field: String,
    pub big_field: Option<[u8; 32]>,

    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}

#[account]
pub struct TestStructData {
    pub field: Option<i32>,
    pub oracle_id: [u8; 32],
    pub oracle_ids: [[u8; 32]; 32],
    pub account_struct: AccountStruct,
    pub accounts: Vec<Vec<u8>>,
    pub different_field: String,
    pub big_field: Option<[u8; 32]>,
    pub nested_dynamic_struct: MidLevelDynamicTestStruct,
    pub nested_static_struct: MidLevelStaticTestStruct,
}

#[account]
pub struct AccountStruct {
    pub account: Vec<u8>,
    pub account_str: String,
}

#[account]
pub struct MidLevelDynamicTestStruct {
    pub fixed_bytes: [u8; 2],
    pub inner: InnerDynamicTestStruct,
}

#[account]
pub struct InnerDynamicTestStruct {
    pub i: i64,
    pub s: String,
}

#[account]
pub struct MidLevelStaticTestStruct {
    pub fixed_bytes: [u8; 2],
    pub inner: InnerStaticTestStruct,
}

#[account]
pub struct InnerStaticTestStruct {
    pub i: i64,
    pub a: Vec<u8>,
}
