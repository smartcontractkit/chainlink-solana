use anchor_lang::prelude::*;

declare_id!("J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4");

pub mod event;

#[program]
pub mod log_read_test {
    use super::*;

    pub fn create_log(_ctx: Context<Initialization>, value: u64) -> Result<()> {
        emit!(event::TestEvent {
            str_val: "Hello, World!".to_string(),
            u64_value: value,
        });

        Ok(())
    }

    pub fn create_truncated_log(_ctx: Context<Initialization>, value: u64) -> Result<()> {
        for _i in 0..100 {
            msg!("Filler message to take up log space: 00000000000000000000000000000000000000000000000000000000000000000000");
        }
        emit!(event::TestEvent {
            str_val: "Hello, World!".to_string(),
            u64_value: value,
        });

        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialization<'info> {
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}
