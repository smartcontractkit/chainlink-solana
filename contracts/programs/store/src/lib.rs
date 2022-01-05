use anchor_lang::prelude::*;

use access_controller::AccessController;

mod state;

use crate::state::with_store;
pub use crate::state::{Store, Transmission, Transmissions};

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
        let mut store = ctx.accounts.store.load_init()?;
        store.owner = ctx.accounts.owner.key();
        store.lowering_access_controller = ctx.accounts.lowering_access_controller.key();
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn create_feed(
        ctx: Context<CreateFeed>,
        granularity: u8,
        live_length: u32,
    ) -> ProgramResult {
        let feed = &mut ctx.accounts.feed;
        feed.version = 1;
        feed.store = ctx.accounts.store.key();
        feed.granularity = granularity;
        feed.live_length = live_length;
        feed.writer = Pubkey::default();
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn set_validator_config(
        ctx: Context<SetValidatorConfig>,
        flagging_threshold: u32,
    ) -> ProgramResult {
        // Check that the feed is owned by the store
        require!(
            ctx.accounts.store.key() == ctx.accounts.feed.store,
            Unauthorized
        );
        ctx.accounts.feed.flagging_threshold = flagging_threshold;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn set_writer(ctx: Context<SetValidatorConfig>, writer: Pubkey) -> ProgramResult {
        // Check that the feed is owned by the store
        require!(
            ctx.accounts.store.key() == ctx.accounts.feed.store,
            Unauthorized
        );
        ctx.accounts.feed.writer = writer;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> ProgramResult {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        let store = &mut *ctx.accounts.store.load_mut()?;
        store.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> ProgramResult {
        let store = &mut *ctx.accounts.store.load_mut()?;
        require!(
            ctx.accounts.authority.key == &store.proposed_owner,
            Unauthorized
        );
        store.owner = std::mem::take(&mut store.proposed_owner);
        Ok(())
    }

    pub fn submit(ctx: Context<Submit>, round: Transmission) -> ProgramResult {
        let mut store = ctx.accounts.store.load_mut()?;

        // check if this particular ocr2 cluster is allowed to write to the feed
        require!(
            ctx.accounts.authority.key == &ctx.accounts.feed.writer,
            Unauthorized
        );

        let previous_round = with_store(&mut ctx.accounts.feed, |store| {
            let previous = store.latest();
            store.insert(round);
            previous
        })?;

        let flagging_threshold = ctx.accounts.feed.flagging_threshold;

        let is_valid = if let Some(previous_round) = previous_round {
            is_valid(flagging_threshold, previous_round.answer, round.answer)
        } else {
            true
        };
        let feed_address = ctx.accounts.feed.key();

        if is_valid {
            // raise flag if not raised yet

            // if the len reaches array len, we're at capacity
            require!(store.flags.remaining_capacity() > 0, Full);

            match store.flags.binary_search(&feed_address) {
                // already present
                Ok(_i) => (),
                // not found, raise flag
                Err(i) => {
                    store.flags.insert(i, feed_address);
                }
            }
        }

        Ok(())
    }

    pub fn lower_flags(ctx: Context<SetFlag>, flags: Vec<Pubkey>) -> ProgramResult {
        let mut store = ctx.accounts.store.load_mut()?;
        has_lowering_access(
            &store,
            &ctx.accounts.access_controller,
            &ctx.accounts.authority,
        )?;

        let positions: Vec<_> = flags
            .iter()
            .filter_map(|flag| store.flags.binary_search(flag).ok())
            .collect();
        for index in positions.iter().rev() {
            // reverse so that subsequent positions aren't affected
            store.flags.remove(*index);
        }
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn set_lowering_access_controller(ctx: Context<SetAccessController>) -> ProgramResult {
        let mut store = ctx.accounts.store.load_mut()?;
        store.lowering_access_controller = ctx.accounts.access_controller.key();
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
fn owner<'info>(state: &AccountLoader<'info, Store>, signer: &'_ AccountInfo) -> ProgramResult {
    require!(signer.key.eq(&state.load()?.owner), Unauthorized);
    Ok(())
}

fn has_lowering_access(
    state: &Store,
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
    pub store: AccountLoader<'info, Store>,
    #[account(signer)]
    pub owner: AccountInfo<'info>,

    pub lowering_access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct CreateFeed<'info> {
    pub store: AccountLoader<'info, Store>,
    #[account(zero)]
    pub feed: Account<'info, Transmissions>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, Store>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, Store>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct SetFlag<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, Store>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct SetValidatorConfig<'info> {
    pub store: AccountLoader<'info, Store>,
    pub authority: Signer<'info>,
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, Store>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct Submit<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, Store>,
    pub authority: Signer<'info>,
    /// The OCR2 feed
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
}
