use anchor_lang::prelude::*;

use arrayvec::arrayvec;

use access_controller::AccessController;

declare_id!("HsCP1ZeNq38jFjMJwSNg3K4U7mkZT2Hc23qyqNf9m88k");

static THRESHOLD_MULTIPLIER: u128 = 100000;

#[zero_copy]
pub struct Flags {
    xs: [Pubkey; 128], // sadly we can't use const https://github.com/project-serum/anchor/issues/632
    len: u64,
}

arrayvec!(Flags, Pubkey, u64);

#[account(zero_copy)]
pub struct Validator {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub raising_access_controller: Pubkey,
    pub lowering_access_controller: Pubkey,

    pub flags: Flags,
}

#[program]
pub mod deviation_flagging_validator {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_init()?;
        state.owner = ctx.accounts.owner.key();
        state.raising_access_controller = ctx.accounts.raising_access_controller.key();
        state.lowering_access_controller = ctx.accounts.lowering_access_controller.key();
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

    pub fn validate(
        ctx: Context<Validate>,
        flagging_threshold: u32,
        _previous_round_id: u32,
        previous_answer: i128,
        _round_id: u32,
        answer: i128,
    ) -> ProgramResult {
        anchor_lang::solana_program::log::sol_log_compute_units();
        let mut state = ctx.accounts.state.load_mut()?;
        has_raising_access(
            &state,
            &ctx.accounts.access_controller,
            &ctx.accounts.authority,
        )?;
        let is_valid = is_valid(flagging_threshold, previous_answer, answer);
        let address = ctx.accounts.address.key();

        if is_valid {
            // raise flag if not raised yet
            let found = state.flags.iter().any(|flag| flag == &address);

            if !found {
                // if the len reaches array len, we're at capacity
                require!(state.flags.remaining_capacity() > 0, Full);
                state.flags.push(address);
            }
        }

        anchor_lang::solana_program::log::sol_log_compute_units();
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
    pub fn set_raising_access_controller(ctx: Context<SetAccessController>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.raising_access_controller = ctx.accounts.access_controller.key();
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

fn has_raising_access(
    // state: &AccountLoader<Validator>,
    state: &Validator,
    controller: &AccountLoader<AccessController>,
    authority: &AccountInfo,
) -> ProgramResult {
    // let state = state.load()?;

    require!(
        state.raising_access_controller == controller.key(),
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

    pub raising_access_controller: AccountLoader<'info, AccessController>,
    pub lowering_access_controller: AccountLoader<'info, AccessController>,
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
pub struct SetFlaggingThreshold<'info> {
    #[account(mut, has_one = owner)]
    pub state: AccountLoader<'info, Validator>,
    pub owner: Signer<'info>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct Validate<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, Validator>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,

    pub address: AccountInfo<'info>,
}
