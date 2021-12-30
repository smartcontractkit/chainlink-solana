use anchor_lang::prelude::*;

use access_controller::AccessController;

mod state;

use crate::state::with_store;
pub use crate::state::{Transmission, Transmissions, Validator};

#[cfg(feature = "mainnet")]
declare_id!("My11111111111111111111111111111111111111113");
#[cfg(feature = "testnet")]
declare_id!("My11111111111111111111111111111111111111113");
#[cfg(feature = "devnet")]
declare_id!("My11111111111111111111111111111111111111113");
#[cfg(not(any(feature = "mainnet", feature = "testnet", feature = "devnet")))]
declare_id!("A7Jh2nb1hZHwqEofm4N8SXbKTj82rx7KUfjParQXUyMQ");

static THRESHOLD_MULTIPLIER: u128 = 100000;

#[program]
pub mod store {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_init()?;
        state.owner = ctx.accounts.owner.key();
        state.lowering_access_controller = ctx.accounts.lowering_access_controller.key();
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn create_feed(
        ctx: Context<CreateFeed>,
        granularity: u8,
        live_length: u32,
    ) -> ProgramResult {
        let store = &mut ctx.accounts.store;
        store.version = 1;
        store.state = ctx.accounts.state.key();
        store.granularity = granularity;
        store.live_length = live_length;
        store.writer = Pubkey::default();
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_validator_config(
        ctx: Context<SetValidatorConfig>,
        flagging_threshold: u32,
    ) -> ProgramResult {
        // Check that the store is owned by the state
        require!(
            ctx.accounts.state.key() == ctx.accounts.store.state,
            Unauthorized
        );
        ctx.accounts.store.flagging_threshold = flagging_threshold;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_writer(ctx: Context<SetValidatorConfig>, writer: Pubkey) -> ProgramResult {
        // Check that the store is owned by the state
        require!(
            ctx.accounts.state.key() == ctx.accounts.store.state,
            Unauthorized
        );
        ctx.accounts.store.writer = writer;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> ProgramResult {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        let state = &mut *ctx.accounts.state.load_mut()?;
        state.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        require!(
            ctx.accounts.authority.key == &state.proposed_owner,
            Unauthorized
        );
        state.owner = std::mem::take(&mut state.proposed_owner);
        Ok(())
    }

    pub fn submit(ctx: Context<Submit>, round: Transmission) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;

        // check if this particular ocr2 cluster is allowed to write to the feed
        require!(
            ctx.accounts.authority.key == &ctx.accounts.store.writer,
            Unauthorized
        );

        let previous_round = with_store(&mut ctx.accounts.store, |store| {
            let previous = store.latest();
            store.insert(round);
            previous
        })?;

        let flagging_threshold = ctx.accounts.store.flagging_threshold;

        let is_valid = if let Some(previous_round) = previous_round {
            is_valid(flagging_threshold, previous_round.answer, round.answer)
        } else {
            true
        };
        let store_address = ctx.accounts.store.key();

        if is_valid {
            // raise flag if not raised yet

            // TODO: use binary search here
            let found = state.flags.iter().any(|flag| flag == &store_address);
            if !found {
                // if the len reaches array len, we're at capacity
                require!(state.flags.remaining_capacity() > 0, Full);
                // TODO insert via binary search
                state.flags.push(store_address);
            }
        }

        Ok(())
    }

    pub fn lower_flags(ctx: Context<SetFlag>, flags: Vec<Pubkey>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        has_lowering_access(
            &state,
            &ctx.accounts.access_controller,
            &ctx.accounts.authority,
        )?;

        // TODO: can probably be improved
        let positions: Vec<_> = state
            .flags
            .iter()
            .enumerate()
            .filter_map(|(i, flag)| flags.contains(flag).then(|| i))
            .collect();
        for index in positions.iter().rev() {
            // reverse so that subsequent positions aren't affected
            state.flags.remove(*index);
        }
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_lowering_access_controller(ctx: Context<SetAccessController>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.lowering_access_controller = ctx.accounts.access_controller.key();
        Ok(())
    }
}

fn is_valid(flagging_threshold: u32, previous_answer: i128, answer: i128) -> bool {
    if previous_answer == 0i128 {
        return true;
    }

    // https://github.com/rust-lang/rust/issues/89492
    fn abs_diff(slf: i128, other: i128) -> u128 {
        if slf < other {
            (other as u128).wrapping_sub(slf as u128)
        } else {
            (slf as u128).wrapping_sub(other as u128)
        }
    }
    let change = abs_diff(previous_answer, answer);
    let ratio_numerator = match change.checked_mul(THRESHOLD_MULTIPLIER) {
        Some(ratio_numerator) => ratio_numerator,
        None => return false,
    };
    let ratio = ratio_numerator / previous_answer.unsigned_abs();
    ratio <= u128::from(flagging_threshold)
}

// Only owner access
fn owner<'info>(state: &AccountLoader<'info, Validator>, signer: &'_ AccountInfo) -> ProgramResult {
    require!(signer.key.eq(&state.load()?.owner), Unauthorized);
    Ok(())
}

fn has_lowering_access(
    state: &Validator,
    controller: &AccountLoader<AccessController>,
    authority: &AccountInfo,
) -> ProgramResult {
    require!(
        state.lowering_access_controller == controller.key(),
        InvalidInput
    );

    let is_owner = state.owner == authority.key();

    let has_access = is_owner
        || access_controller::has_access(controller, authority.key)
            // TODO: better mapping, maybe InvalidInput?
            .map_err(|_| ErrorCode::Unauthorized)?;

    require!(has_access, Unauthorized);
    Ok(())
}

#[error]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized = 0,

    #[msg("Invalid input")]
    InvalidInput = 1,

    #[msg("Flags list is full")]
    Full = 2,
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(zero)]
    pub state: AccountLoader<'info, Validator>,
    #[account(signer)]
    pub owner: AccountInfo<'info>,

    pub lowering_access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct CreateFeed<'info> {
    pub state: AccountLoader<'info, Validator>,
    #[account(zero)]
    pub store: Account<'info, Transmissions>, // TODO: use (init with payer)
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct SetFlag<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct SetValidatorConfig<'info> {
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
    #[account(mut)]
    pub store: Account<'info, Transmissions>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct Submit<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,

    /// The OCR2 feed
    #[account(mut)]
    pub store: Account<'info, Transmissions>,
}
