use anchor_lang::prelude::*;
use anchor_lang::solana_program::system_program;

use chainlink_solana as chainlink;

declare_id!("EsYPTcY4Be6GvxojV5kwZ7W2tK2hoVkm9XSN7Lk8HAs8");

#[account]
pub struct Decimal {
    pub value: i128,
    pub decimals: u32,
}

impl Decimal {
    pub fn new(value: i128, decimals: u32) -> Self {
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

#[program]
pub mod chainlink_solana_demo {
    use super::*;
    //pub fn execute(ctx: Context<Execute>) -> Result<String,anchor_lang::prelude::ProgramError> {
        pub fn execute(ctx: Context<Execute>) -> ProgramResult  {
        let round = chainlink::latest_round_data(
            ctx.accounts.chainlink_program.to_account_info(),
            ctx.accounts.chainlink_feed.to_account_info(),
        )?;

        let description = chainlink::description(
            ctx.accounts.chainlink_program.to_account_info(),
            ctx.accounts.chainlink_feed.to_account_info(),
        )?;

        let decimals = chainlink::decimals(
            ctx.accounts.chainlink_program.to_account_info(),
            ctx.accounts.chainlink_feed.to_account_info(),
        )?;

        //let decimal = Decimal::new(round.answer, u32::from(decimals));
        //set the account value
        let decimal: &mut Account<Decimal> = &mut ctx.accounts.decimal;
        decimal.value=round.answer;
        decimal.decimals=u32::from(decimals);

        let decimalPrint = Decimal::new(round.answer, u32::from(decimals));
        msg!("{} price is {}", description, decimalPrint);



        Ok(())

        //return Err("The error message".to_string());
        //Ok(decimal.to_string())

    }
}

#[derive(Accounts)]
pub struct Execute<'info> {
    #[account(init, payer = user, space = 100)]
    pub decimal: Account<'info, Decimal>,
    #[account(mut)]
    pub user: Signer<'info>,
    pub chainlink_feed: AccountInfo<'info>,
    pub chainlink_program: AccountInfo<'info>,
    #[account(address = system_program::ID)]
    pub system_program: AccountInfo<'info>,
}
