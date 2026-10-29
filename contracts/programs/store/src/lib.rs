#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
use anchor_lang::prelude::*;

use access_controller::AccessController;

mod state;

use crate::state::with_store;
pub use crate::state::{
    NewTransmission, OnReportCallbackArgs, Store as State, Transmission, Transmissions,
};

declare_id!("9kRNTZmoZSiTBuXC62dzK9E7gC7huYgcmRRhYv3i4osC");

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

/// Why `on_report_callback` left a feed untouched.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq, Eq)]
pub enum CacheSkipReason {
    /// Not a store-owned `Transmissions` account.
    InvalidFeedAccount,
    /// Feed account was not passed as writable.
    NotWritable,
    /// Feed version does not match `FEED_VERSION`.
    InvalidVersion,
    /// The feed's writer is not the signing data feeds store callback authority (e.g. still OCR2).
    UnauthorizedWriter,
    /// Round is not newer than the feed's latest round.
    StaleTimestamp,
    /// Answer does not fit once rescaled to the feed's decimals.
    AnswerOverflow,
}

#[event]
pub struct CacheRoundSubmitted {
    pub feed: Pubkey,
    pub round_id: u32,
    pub answer: i128,
    pub timestamp: u32,
}

#[event]
pub struct CacheRoundSkipped {
    pub feed: Pubkey,
    pub reason: CacheSkipReason,
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
        require!(
            len % size_of::<Transmission>() == 0,
            ErrorCode::InsufficientSize
        );
        let space = len / size_of::<Transmission>();
        // Live length must not exceed total capacity
        require!(live_length <= space as u32, ErrorCode::InvalidInput);

        // Both inputs should also be more than zero
        require!(live_length > 0, ErrorCode::InvalidInput);
        require!(granularity > 0, ErrorCode::InvalidInput);

        feed.version = FEED_VERSION;
        feed.state = Transmissions::NORMAL;
        feed.owner = ctx.accounts.authority.key();
        feed.granularity = granularity;
        feed.live_length = live_length;
        feed.writer = Pubkey::default();

        feed.decimals = decimals;
        let description = description.as_bytes();
        require!(description.len() <= 32, ErrorCode::InvalidInput);
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
        ctx.accounts.feed.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_feed_ownership(ctx: Context<AcceptFeedOwnership>) -> Result<()> {
        let store: std::result::Result<AccountLoader<State>, _> =
            try_from!(AccountLoader<State>, &ctx.accounts.proposed_owner);

        let proposed_owner = match store {
            // if the feed is owned by a store, validate the store's owner signed
            Ok(store) => store.load()?.owner,
            // else, it's an individual owner
            Err(_err) => ctx.accounts.proposed_owner.key(),
        };
        require!(
            ctx.accounts.authority.key == &proposed_owner,
            ErrorCode::Unauthorized
        );

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
        let clock = Clock::get()?;
        let round = Transmission {
            slot: clock.slot,
            answer: round.answer,
            timestamp: round.timestamp as u32,
            ..Default::default()
        };

        apply_round(&mut ctx.accounts.feed, round)
    }

    /// Report callback of the data feeds store (CRE), configured on the data ID backing this feed.
    /// Writes the round, rescaled to the feed's decimals, if the feed's writer is the signing
    /// authority: the data feeds store's callback authority PDA for this program. A round the feed
    /// cannot take (e.g. its writer is still OCR2) is skipped with a `CacheRoundSkipped` event.
    /// Always accepts, so the store keeps its own write. Its events are CPI events (`emit_cpi!`).
    pub fn on_report_callback(
        ctx: Context<OnReportCallback>,
        args: OnReportCallbackArgs,
    ) -> Result<bool> {
        let feed_info = ctx.accounts.feed.to_account_info();
        let skip = |reason| -> Result<bool> {
            emit_cpi!(CacheRoundSkipped {
                feed: feed_info.key(),
                reason,
            });
            Ok(true)
        };

        if !feed_info.is_writable {
            return skip(CacheSkipReason::NotWritable);
        }

        let feed_account: std::result::Result<Account<Transmissions>, _> =
            try_from!(Account<Transmissions>, &ctx.accounts.feed);
        let mut feed = match feed_account {
            Ok(feed) => feed,
            Err(_) => return skip(CacheSkipReason::InvalidFeedAccount),
        };

        let slot = Clock::get()?.slot;
        let authority = ctx.accounts.authority.key();
        let transmission = match prepare_cache_round(&mut feed, &authority, &args, slot)? {
            Ok(transmission) => transmission,
            Err(reason) => return skip(reason),
        };

        apply_round(&mut feed, transmission)?;
        // the feed is not a typed account of this context, persist explicitly
        feed.exit(&crate::ID)?;

        emit_cpi!(CacheRoundSubmitted {
            feed: feed_info.key(),
            round_id: feed.latest_round_id,
            answer: transmission.answer,
            timestamp: transmission.timestamp,
        });
        Ok(true)
    }

    // Store methods

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        let mut store = ctx.accounts.store.load_init()?;
        store.owner = ctx.accounts.owner.key();
        store.lowering_access_controller = ctx.accounts.lowering_access_controller.key();
        Ok(())
    }

    pub fn transfer_store_ownership(
        ctx: Context<TransferStoreOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        let store = &mut *ctx.accounts.store.load_mut()?;
        store.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_store_ownership(ctx: Context<AcceptStoreOwnership>) -> Result<()> {
        let store = &mut *ctx.accounts.store.load_mut()?;
        store.owner = std::mem::take(&mut store.proposed_owner);
        Ok(())
    }

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

/// Insert a round into the feed and raise the flag if it deviates past the flagging threshold.
fn apply_round(feed: &mut Account<Transmissions>, round: Transmission) -> Result<()> {
    let previous_round = with_store(feed, |store| {
        let previous = store.latest();
        store.insert(round);
        previous
    })?;

    let is_valid = if let Some(previous_round) = previous_round {
        is_valid(feed.flagging_threshold, previous_round.answer, round.answer)
    } else {
        true
    };

    if !is_valid {
        // raise flag
        feed.state = Transmissions::FLAGGED;
    }

    Ok(())
}

/// Validate a cache round against the feed and convert it to the feed's decimals.
/// The inner `Err` is a skip reason, the outer one a hard failure.
fn prepare_cache_round(
    feed: &mut Account<Transmissions>,
    authority: &Pubkey,
    round: &OnReportCallbackArgs,
    slot: u64,
) -> Result<std::result::Result<Transmission, CacheSkipReason>> {
    if feed.version != FEED_VERSION {
        return Ok(Err(CacheSkipReason::InvalidVersion));
    }

    if feed.writer != *authority {
        return Ok(Err(CacheSkipReason::UnauthorizedWriter));
    }

    let latest = with_store(feed, |store| store.latest())?;
    if let Some(latest) = latest {
        if round.observations_timestamp <= latest.timestamp {
            return Ok(Err(CacheSkipReason::StaleTimestamp));
        }
    }

    let Some(answer) = rescale(round.answer, round.decimals, feed.decimals) else {
        return Ok(Err(CacheSkipReason::AnswerOverflow));
    };

    Ok(Ok(Transmission {
        slot,
        answer,
        timestamp: round.observations_timestamp,
        ..Default::default()
    }))
}

/// Convert `answer` from `from` to `to` decimals. Downscaling truncates toward zero.
fn rescale(answer: i128, from: u8, to: u8) -> Option<i128> {
    use std::cmp::Ordering;

    match from.cmp(&to) {
        Ordering::Equal => Some(answer),
        Ordering::Greater => match 10i128.checked_pow(u32::from(from - to)) {
            Some(divisor) => Some(answer / divisor),
            // the divisor exceeds any i128 answer
            None => Some(0),
        },
        Ordering::Less => 10i128
            .checked_pow(u32::from(to - from))
            .and_then(|multiplier| answer.checked_mul(multiplier)),
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

// https://github.com/coral-xyz/anchor/pull/2770
#[macro_export]
macro_rules! try_from {
    ($ty: ty, $acc: expr) => {{
        let acc_ref = $acc.as_ref();
        <$ty>::try_from(unsafe {
            core::mem::transmute::<&AccountInfo<'_>, &AccountInfo<'_>>(acc_ref)
        })
    }};
}

// Only owner access
fn owner<'info>(owner: &UncheckedAccount<'info>, authority: &Signer) -> Result<()> {
    let store: std::result::Result<AccountLoader<'info, State>, _> =
        try_from!(AccountLoader<'info, State>, owner);

    let owner = match store {
        // if the feed is owned by a store, validate the store's owner signed
        Ok(store) => store.load()?.owner,
        // else, it's an individual owner
        Err(_err) => *owner.key,
    };

    require!(authority.key == &owner, ErrorCode::Unauthorized);
    Ok(())
}

fn has_lowering_access(
    owner: &UncheckedAccount,
    controller: &UncheckedAccount,
    authority: &Signer,
) -> Result<()> {
    let store: std::result::Result<AccountLoader<State>, _> =
        try_from!(AccountLoader<State>, owner);

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
                ErrorCode::InvalidInput
            );

            let controller = try_from!(AccountLoader<AccessController>, controller)?;

            // Check if the key is present on the access controller
            let has_access = access_controller::has_access(&controller, authority.key)
                // TODO: better mapping, maybe InvalidInput?
                .map_err(|_| ErrorCode::Unauthorized)?;

            require!(has_access, ErrorCode::Unauthorized);
        }
        // else, it's an individual owner
        Err(_err) => {
            require!(authority.key == owner.key, ErrorCode::Unauthorized);
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
    // check if this particular ocr2 cluster is allowed to write to the feed
    #[account(address = feed.writer @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct OnReportCallback<'info> {
    /// The data feeds store's callback authority PDA of this program (`[b"callback_authority",
    /// store state, this program]` under the data feeds store). Each feed only accepts rounds if
    /// this is its writer.
    pub authority: Signer<'info>,
    /// CHECK: the legacy feed, validated in the handler so a bad feed is skipped, not failed
    pub feed: UncheckedAccount<'info>,
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
    #[account(address = store.load()?.owner @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptStoreOwnership<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, State>,
    #[account(address = store.load()?.proposed_owner @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub store: AccountLoader<'info, State>,
    #[account(address = store.load()?.owner @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct Query<'info> {
    pub feed: Account<'info, Transmissions>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::mem::size_of;

    const LIVE_LENGTH: usize = 2;
    const FEED_LEN: usize = 8 + state::HEADER_SIZE + LIVE_LENGTH * size_of::<Transmission>();

    fn write_feed(data: &mut [u8], writer: Pubkey, decimals: u8) {
        let mut cursor = std::io::Cursor::new(&mut data[..8 + state::HEADER_SIZE]);
        Transmissions {
            version: FEED_VERSION,
            state: Transmissions::NORMAL,
            owner: Pubkey::default(),
            proposed_owner: Pubkey::default(),
            writer,
            description: [0; 32],
            decimals,
            flagging_threshold: 1000,
            latest_round_id: 0,
            granularity: 1,
            live_length: LIVE_LENGTH as u32,
            live_cursor: 0,
            historical_cursor: 0,
        }
        .try_serialize(&mut cursor)
        .unwrap();
    }

    fn cache_round(timestamp: u32, answer: i128) -> OnReportCallbackArgs {
        OnReportCallbackArgs {
            data_id: [1; 16],
            round_id: 1,
            answer,
            decimals: 18,
            observations_timestamp: timestamp,
            recorded_timestamp: timestamp,
        }
    }

    #[test]
    fn rescale_converts_between_decimals() {
        // 18 -> 8 truncates toward zero, like the EVM DataFeedsStore downscale
        assert_eq!(
            rescale(123_456_789_999_999_999_999, 18, 8),
            Some(12_345_678_999)
        );
        assert_eq!(
            rescale(-123_456_789_999_999_999_999, 18, 8),
            Some(-12_345_678_999)
        );
        assert_eq!(rescale(42, 8, 8), Some(42));
        assert_eq!(rescale(42, 6, 8), Some(4_200));
        assert_eq!(rescale(i128::MAX, 0, 18), None);
        assert_eq!(rescale(i128::MAX, 255, 0), Some(0));
    }

    #[test]
    fn prepare_cache_round_validates_feed() {
        let key = Pubkey::new_unique();
        let writer = Pubkey::new_unique();
        let owner = crate::ID;
        let mut lamports = 0u64;
        let mut backing = Vec::new();
        let data = state::aligned_account_data(&mut backing, FEED_LEN);
        write_feed(data, writer, 8);
        let info = AccountInfo::new(&key, false, true, &mut lamports, data, &owner, false, 0);
        let mut feed = Account::<Transmissions>::try_from(&info).unwrap();

        // not the writer, e.g. the feed still points at OCR2
        let other = Pubkey::new_unique();
        assert_eq!(
            prepare_cache_round(&mut feed, &other, &cache_round(10, 1), 1).unwrap(),
            Err(CacheSkipReason::UnauthorizedWriter)
        );

        // rescaled to the feed's decimals
        let round =
            prepare_cache_round(&mut feed, &writer, &cache_round(10, 2 * 10i128.pow(18)), 7)
                .unwrap()
                .unwrap();
        assert_eq!(round.answer, 2 * 10i128.pow(8));
        assert_eq!(round.timestamp, 10);
        assert_eq!(round.slot, 7);
        apply_round(&mut feed, round).unwrap();
        assert_eq!(feed.latest_round_id, 1);

        // not newer than the latest round
        assert_eq!(
            prepare_cache_round(&mut feed, &writer, &cache_round(10, 1), 8).unwrap(),
            Err(CacheSkipReason::StaleTimestamp)
        );

        // does not fit once upscaled
        let overflow = OnReportCallbackArgs {
            answer: i128::MAX,
            decimals: 0,
            ..cache_round(11, 0)
        };
        assert_eq!(
            prepare_cache_round(&mut feed, &writer, &overflow, 8).unwrap(),
            Err(CacheSkipReason::AnswerOverflow)
        );

        feed.version = FEED_VERSION - 1;
        assert_eq!(
            prepare_cache_round(&mut feed, &writer, &cache_round(11, 1), 8).unwrap(),
            Err(CacheSkipReason::InvalidVersion)
        );
    }

    #[test]
    fn apply_round_flags_deviation() {
        let key = Pubkey::new_unique();
        let owner = crate::ID;
        let mut lamports = 0u64;
        let mut backing = Vec::new();
        let data = state::aligned_account_data(&mut backing, FEED_LEN);
        write_feed(data, Pubkey::default(), 8);
        let info = AccountInfo::new(&key, false, true, &mut lamports, data, &owner, false, 0);
        let mut feed = Account::<Transmissions>::try_from(&info).unwrap();

        let round = |timestamp, answer| Transmission {
            slot: 1,
            timestamp,
            answer,
            ..Default::default()
        };

        // 1% threshold (1000 / 100000)
        apply_round(&mut feed, round(1, 100_000)).unwrap();
        apply_round(&mut feed, round(2, 100_500)).unwrap();
        assert_eq!(feed.state, Transmissions::NORMAL);
        apply_round(&mut feed, round(3, 200_000)).unwrap();
        assert_eq!(feed.state, Transmissions::FLAGGED);
        assert_eq!(feed.latest_round_id, 3);
    }
}
