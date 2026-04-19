use anchor_lang::prelude::*;

use chainlink_solana::v2::read_feed_v2;

declare_id!("2zqDPfT7nqvL7NrptW13BVmaXcZqyin7RubAGfxEK1om");

struct Decimal {
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
pub mod hello_world {
    use super::*;
    pub fn execute(ctx: Context<Execute>) -> Result<()> {
        require_keys_eq!(
            *ctx.accounts.chainlink_feed.owner,
            *ctx.accounts.chainlink_program.key,
            ErrorCode::InvalidChainlinkProgram
        );

        let feed = read_feed_v2(
            ctx.accounts.chainlink_feed.try_borrow_data()?,
            ctx.accounts.chainlink_feed.owner.to_bytes(),
            &ctx.accounts.chainlink_program.key(),
        )
        .map_err(|_| error!(ErrorCode::FeedReadFailed))?;

        let round = feed
            .latest_round_data()
            .ok_or(error!(ErrorCode::NoRoundData))?;

        let raw = feed.description();
        let end = raw.iter().position(|&b| b == 0).unwrap_or(raw.len());
        let description = core::str::from_utf8(&raw[..end]).map_err(|_| error!(ErrorCode::BadDescription))?;

        let decimal = Decimal::new(round.answer, u32::from(feed.decimals()));
        msg!("{} price is {}", description, decimal);
        Ok(())
    }
}

#[error_code]
pub enum ErrorCode {
    #[msg("Feed account owner must match the Chainlink store program")]
    InvalidChainlinkProgram,
    #[msg("Failed to read Chainlink feed account")]
    FeedReadFailed,
    #[msg("Feed has no round data")]
    NoRoundData,
    #[msg("Feed description is not valid UTF-8")]
    BadDescription,
}

#[derive(Accounts)]
pub struct Execute<'info> {
    /// CHECK:
    pub chainlink_feed: AccountInfo<'info>,
    /// CHECK: TODO: add Anchor types to chainlink-solana
    pub chainlink_program: AccountInfo<'info>,
}
