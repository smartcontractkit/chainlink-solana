use anchor_lang::prelude::*;

use access_controller::AccessController;

mod state;

use crate::state::with_store;
pub use crate::state::{NewTransmission, Store as State, Transmission, Transmissions};

declare_id!("HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny");

static THRESHOLD_MULTIPLIER: u128 = 100000;

const FEED_VERSION: u8 = 2;

#[derive(Clone)]
pub struct Store;

impl anchor_lang::Id for Store {
    fn id() -> Pubkey {
        ID
    }
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub enum Scope {
    Version,
    Decimals,
    Description,
    RoundData { round_id: u32 },
    LatestRoundData,
    Aggregator,
    // ProposedAggregator
    // Owner
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct Round {
    pub round_id: u32,
    pub slot: u64,
    pub timestamp: u32,
    pub answer: i128,
}

#[program]
pub mod store {
    use super::*;

    // Feed methods

    pub fn create_feed(
        ctx: Context<CreateFeed>,
        description: String,
        decimals: u8,
        granularity: u8,
        live_length: u32,
    ) -> Result<()> {
        use std::mem::size_of;

        let feed = &mut ctx.accounts.feed;

        // Validate the feed account is of the correct size
        let len = feed.to_account_info().data_len();
        // discriminator + header size
        let len = len
            .checked_sub(8 + state::HEADER_SIZE)
            .ok_or(ErrorCode::InsufficientSize)?;
        require!(len % size_of::<Transmission>() == 0, InsufficientSize);
        let space = len / size_of::<Transmission>();
        // Live length must not exceed total capacity
        require!(live_length <= space as u32, InvalidInput);

        // Both inputs should also be more than zero
        require!(live_length > 0, InvalidInput);
        require!(granularity > 0, InvalidInput);

        feed.version = FEED_VERSION;
        feed.state = Transmissions::NORMAL;
        feed.owner = ctx.accounts.authority.key();
        feed.granularity = granularity;
        feed.live_length = live_length;
        feed.writer = Pubkey::default();

        feed.decimals = decimals;
        let description = description.as_bytes();
        require!(description.len() <= 32, InvalidInput);
        feed.description[..description.len()].copy_from_slice(description);

        Ok(())
    }

    #[access_control(owner(&ctx.accounts.owner, &ctx.accounts.authority))]
    pub fn close_feed(ctx: Context<CloseFeed>) -> Result<()> {
        // NOTE: Close is handled by anchor on exit due to the `close` attribute
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.owner, &ctx.accounts.authority))]
    pub fn transfer_feed_ownership(
        ctx: Context<TransferFeedOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        ctx.accounts.feed.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_feed_ownership(ctx: Context<AcceptFeedOwnership>) -> Result<()> {
        let store: std::result::Result<AccountLoader<State>, _> =
            AccountLoader::try_from(&ctx.accounts.proposed_owner);

        let proposed_owner = match store {
            // if the feed is owned by a store, validate the store's owner signed
            Ok(store) => store.load()?.owner,
            // else, it's an individual owner
            Err(_err) => ctx.accounts.proposed_owner.key(),
        };
        require!(ctx.accounts.authority.key == &proposed_owner, Unauthorized);

        let feed = &mut ctx.accounts.feed;
        feed.owner = std::mem::take(&mut feed.proposed_owner);
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.owner, &ctx.accounts.authority))]
    pub fn set_validator_config(
        ctx: Context<SetFeedConfig>,
        flagging_threshold: u32,
    ) -> Result<()> {
        ctx.accounts.feed.flagging_threshold = flagging_threshold;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.owner, &ctx.accounts.authority))]
    pub fn set_writer(ctx: Context<SetFeedConfig>, writer: Pubkey) -> Result<()> {
        ctx.accounts.feed.writer = writer;
        Ok(())
    }

    // NOTE: to bulk lower, a batch transaction can be sent with a bunch of lower calls
    #[access_control(has_lowering_access(
            &ctx.accounts.owner,
            &ctx.accounts.access_controller,
            &ctx.accounts.authority,
    ))]
    pub fn lower_flag(ctx: Context<LowerFlag>) -> Result<()> {
        ctx.accounts.feed.state = Transmissions::NORMAL;
        Ok(())
    }

    pub fn submit(ctx: Context<Submit>, round: NewTransmission) -> Result<()> {
        // check if this particular ocr2 cluster is allowed to write to the feed
        require!(
            ctx.accounts.authority.key == &ctx.accounts.feed.writer,
            Unauthorized
        );

        let clock = Clock::get()?;
        let round = Transmission {
            slot: clock.slot,
            answer: round.answer,
            timestamp: round.timestamp as u32,
            ..Default::default()
        };

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

        if !is_valid {
            // raise flag
            ctx.accounts.feed.state = Transmissions::FLAGGED;
        }

        Ok(())
    }

    // Store methods

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        let mut store = ctx.accounts.store.load_init()?;
        store.owner = ctx.accounts.owner.key();
        store.lowering_access_controller = ctx.accounts.lowering_access_controller.key();
        Ok(())
    }

    #[access_control(store_owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn transfer_store_ownership(
        ctx: Context<TransferStoreOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        let store = &mut *ctx.accounts.store.load_mut()?;
        store.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_store_ownership(ctx: Context<AcceptStoreOwnership>) -> Result<()> {
        let store = &mut *ctx.accounts.store.load_mut()?;
        require!(
            ctx.accounts.authority.key == &store.proposed_owner,
            Unauthorized
        );
        store.owner = std::mem::take(&mut store.proposed_owner);
        Ok(())
    }

    #[access_control(store_owner(&ctx.accounts.store, &ctx.accounts.authority))]
    pub fn set_lowering_access_controller(ctx: Context<SetAccessController>) -> Result<()> {
        let mut store = ctx.accounts.store.load_mut()?;
        store.lowering_access_controller = ctx.accounts.access_controller.key();
        Ok(())
    }

    /// The query instruction takes a `Query` and serializes the response in a fixed format. That way queries
    /// are not bound to the underlying layout.
    pub fn query(ctx: Context<Query>, scope: Scope) -> Result<()> {
        use std::io::Cursor;

        let mut buf = Cursor::new(Vec::with_capacity(128)); // TODO: calculate max size
        let header = &ctx.accounts.feed;

        match scope {
            Scope::Version => {
                let data = header.version;
                data.serialize(&mut buf)?;
            }
            Scope::Decimals => {
                let data = header.decimals;
                data.serialize(&mut buf)?;
            }
            Scope::Description => {
                // Look for the first null byte
                let end = header
                    .description
                    .iter()
                    .position(|byte| byte == &0)
                    .unwrap_or(header.description.len());

                let description = String::from_utf8(header.description[..end].to_vec())
                    .map_err(|_err| ErrorCode::InvalidInput)?;

                let data = description;
                data.serialize(&mut buf)?;
            }
            Scope::RoundData { round_id } => {
                let round = with_store(&mut ctx.accounts.feed, |store| store.fetch(round_id))?
                    .ok_or(ErrorCode::NotFound)?;

                let data = Round {
                    round_id,
                    slot: round.slot,
                    answer: round.answer,
                    timestamp: round.timestamp,
                };
                data.serialize(&mut buf)?;
            }
            Scope::LatestRoundData => {
                let round = with_store(&mut ctx.accounts.feed, |store| store.latest())?
                    .ok_or(ErrorCode::NotFound)?;

                let header = &ctx.accounts.feed;

                let data = Round {
                    round_id: header.latest_round_id,
                    slot: round.slot,
                    answer: round.answer,
                    timestamp: round.timestamp,
                };

                data.serialize(&mut buf)?;
            }
            Scope::Aggregator => {
                let data = header.writer;
                data.serialize(&mut buf)?;
            }
        }

        anchor_lang::solana_program::program::set_return_data(buf.get_ref());
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
fn owner<'info>(owner: &UncheckedAccount<'info>, authority: &Signer) -> Result<()> {
    let store: std::result::Result<AccountLoader<'info, State>, _> = AccountLoader::try_from(owner);

    let owner = match store {
        // if the feed is owned by a store, validate the store's owner signed
        Ok(store) => store.load()?.owner,
        // else, it's an individual owner
        Err(_err) => *owner.key,
    };

    require!(authority.key == &owner, Unauthorized);
    Ok(())
}

fn store_owner(store_loader: &AccountLoader<State>, signer: &AccountInfo) -> Result<()> {
    let store = store_loader.load()?;
    require!(signer.key.eq(&store.owner), Unauthorized);
    Ok(())
}

fn has_lowering_access(
    owner: &UncheckedAccount,
    controller: &UncheckedAccount,
    authority: &Signer,
) -> Result<()> {
    let store: std::result::Result<AccountLoader<State>, _> = AccountLoader::try_from(owner);

    match store {
        // if the feed is owned by a store
        Ok(store) => {
            let store = store.load()?;
            let is_owner = store.owner == authority.key();

            // the signer is the store owner, fast path return
            if is_owner {
                return Ok(());
            }

            // else, we check the lowering_access_controller

            // The controller account has to match the lowering_access_controller on the store
            require!(
                controller.key() == store.lowering_access_controller,
                InvalidInput
            );

            let controller = AccountLoader::try_from(controller)?;

            // Check if the key is present on the access controller
            let has_access = access_controller::has_access(&controller, authority.key)
                // TODO: better mapping, maybe InvalidInput?
                .map_err(|_| ErrorCode::Unauthorized)?;

            require!(has_access, Unauthorized);
        }
        // else, it's an individual owner
        Err(_err) => {
            require!(authority.key == owner.key, Unauthorized);
        }
    };

    Ok(())
}

#[cfg(feature = "cpi")]
pub mod accessors {
    use crate::cpi::{self, accounts::Query};
    use crate::{Round, Scope};
    use anchor_lang::prelude::*;
    use anchor_lang::solana_program;

    fn query<'info, T: AnchorDeserialize>(
        program_id: AccountInfo<'info>,
        feed: AccountInfo<'info>,
        scope: Scope,
    ) -> Result<T> {
        let cpi = CpiContext::new(program_id, Query { feed });
        cpi::query(cpi, scope)?;
        let (_key, data) = solana_program::program::get_return_data().unwrap();
        let data = T::try_from_slice(&data)?;
        Ok(data)
    }

    pub fn version<'info>(program_id: AccountInfo<'info>, feed: AccountInfo<'info>) -> Result<u8> {
        query(program_id, feed, Scope::Version)
    }

    pub fn decimals<'info>(program_id: AccountInfo<'info>, feed: AccountInfo<'info>) -> Result<u8> {
        query(program_id, feed, Scope::Decimals)
    }

    pub fn description<'info>(
        program_id: AccountInfo<'info>,
        feed: AccountInfo<'info>,
    ) -> Result<String> {
        query(program_id, feed, Scope::Description)
    }

    pub fn round_data<'info>(
        program_id: AccountInfo<'info>,
        feed: AccountInfo<'info>,
        round_id: u32,
    ) -> Result<Round> {
        query(program_id, feed, Scope::RoundData { round_id })
    }

    pub fn latest_round_data<'info>(
        program_id: AccountInfo<'info>,
        feed: AccountInfo<'info>,
    ) -> Result<Round> {
        query(program_id, feed, Scope::LatestRoundData)
    }

    pub fn aggregator<'info>(
        program_id: AccountInfo<'info>,
        feed: AccountInfo<'info>,
    ) -> Result<Pubkey> {
        query(program_id, feed, Scope::Aggregator)
    }
}

#[error_code]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized = 0,

    #[msg("Invalid input")]
    InvalidInput = 1,

    NotFound = 2,

    #[msg("Invalid version")]
    InvalidVersion = 3,

    #[msg("Insufficient or invalid feed account size, has to be `8 + HEADER_SIZE + n * size_of::<Transmission>()`")]
    InsufficientSize = 4,
}

// Feed methods

#[derive(Accounts)]
pub struct CreateFeed<'info> {
    #[account(zero)]
    pub feed: Account<'info, Transmissions>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct CloseFeed<'info> {
    #[account(mut, close = receiver)]
    pub feed: Account<'info, Transmissions>,
    /// CHECK: through the owner() access_control
    #[account(address = feed.owner)]
    pub owner: UncheckedAccount<'info>,
    #[account(mut)]
    pub receiver: SystemAccount<'info>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct SetFeedConfig<'info> {
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
    /// CHECK: through the owner() access_control
    #[account(address = feed.owner)]
    pub owner: UncheckedAccount<'info>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct LowerFlag<'info> {
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
    /// CHECK: through the has_lowering_access() access_control
    #[account(address = feed.owner)]
    pub owner: UncheckedAccount<'info>,
    pub authority: Signer<'info>,
    /// CHECK: through the has_lowering_access() access_control
    pub access_controller: UncheckedAccount<'info>,
}

#[derive(Accounts)]
pub struct TransferFeedOwnership<'info> {
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
    /// CHECK: through the owner() access_control
    #[account(address = feed.owner)]
    pub owner: UncheckedAccount<'info>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptFeedOwnership<'info> {
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
    /// CHECK: we validate this inside accept_feed_ownership
    #[account(address = feed.proposed_owner)]
    pub proposed_owner: UncheckedAccount<'info>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct Submit<'info> {
    /// The OCR2 feed
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
    pub authority: Signer<'info>,
}

// Store methods

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(zero)]
    pub store: AccountLoader<'info, State>,
    pub owner: Signer<'info>,
    pub lowering_access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct TransferStoreOwnership<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptStoreOwnership<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct Query<'info> {
    pub feed: Account<'info, Transmissions>,
}
