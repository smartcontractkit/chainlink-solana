use solana_program::{
    account_info::{next_account_info, AccountInfo},
    entrypoint,
    entrypoint::ProgramResult,
    msg,
    pubkey::Pubkey,
};

struct Decimal {
    pub value: u128,
    pub decimals: u32,
}

impl Decimal {
    pub fn new(value: u128, decimals: u32) -> Self {
        Decimal { value, decimals }
    }
}

impl std::fmt::Display for Decimal {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let mut scaled_val = self.value.to_string();
        if scaled_val.len() <= self.decimals as usize {
            scaled_val.insert_str(
                0,
                &vec!["0"; self.decimals as usize - scaled_val.len()].join(""),
            );
            scaled_val.insert_str(0, "0.");
        } else {
            scaled_val.insert(scaled_val.len() - self.decimals as usize, '.');
        }
        f.write_str(&scaled_val)
    }
}

// Declare and export the program's entrypoint
entrypoint!(process_instruction);

// Program entrypoint's implementation
pub fn process_instruction(
    _program_id: &Pubkey, // Public key of the account the hello world program was loaded into
    accounts: &[AccountInfo],
    _instruction_data: &[u8],
) -> ProgramResult {
    msg!("Hello World Rust program entrypoint");

    let accounts_iter = &mut accounts.iter();
    let feed_account = next_account_info(accounts_iter)?;

    const DECIMALS: u32 = 9;

    let price = chainlink::get_price(&chainlink::id(), feed_account)?;

    if let Some(price) = price {
        let decimal = Decimal::new(price, DECIMALS);
        msg!("Price is {}", decimal);
    } else {
        msg!("No current price");
    }

    Ok(())
}
