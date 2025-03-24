//! Chainlink Data Streams Client for Solana

mod solana {
    #[cfg(not(target_os = "solana"))]
    pub use solana_sdk::{
        instruction::{AccountMeta, Instruction},
        pubkey::Pubkey,
    };

    #[cfg(target_os = "solana")]
    pub use solana_program::{
        instruction::{AccountMeta, Instruction},
        pubkey::Pubkey,
    };
}

use crate::solana::{AccountMeta, Instruction, Pubkey};
use borsh::{BorshDeserialize, BorshSerialize};

/// Program function name discriminators
pub mod discriminator {
    pub const VERIFY: [u8; 8] = [133, 161, 141, 48, 120, 198, 88, 150];
}

#[derive(BorshSerialize, BorshDeserialize)]
struct VerifyParams {
    signed_report: Vec<u8>,
}

/// A helper struct for creating Verifier program instructions
pub struct VerifierInstructions;

impl VerifierInstructions {
    /// Creates a verify instruction.
    ///
    /// # Parameters:
    ///
    /// * `program_id` - The public key of the verifier program.
    /// * `verifier_account` - The public key of the verifier account. The function [`Self::get_verifier_config_pda`] can be used to calculate this.
    /// * `access_controller_account` - The public key of the access controller account.
    /// * `user` - The public key of the user - this account must be a signer.
    /// * `report_config_account` - The public key of the report configuration account. The function [`Self::get_config_pda`] can be used to calculate this.
    /// * `signed_report` - Report bytes from Data Streams DON compressed in snappy format
    ///
    /// # Returns
    ///
    /// Returns an `Instruction` object that can be sent to the Solana runtime.
    pub fn verify(
        program_id: &Pubkey,
        verifier_account: &Pubkey,
        access_controller_account: &Pubkey,
        user: &Pubkey,
        report_config_account: &Pubkey,
        signed_report: Vec<u8>,
    ) -> Instruction {
        let accounts = vec![
            AccountMeta::new_readonly(*verifier_account, false),
            AccountMeta::new_readonly(*access_controller_account, false),
            AccountMeta::new_readonly(*user, true),
            AccountMeta::new_readonly(*report_config_account, false),
        ];

        // 8 bytes for discriminator
        // 4 bytes size of the length prefix for the signed_report vector
        let mut instruction_data = Vec::with_capacity(8 + 4 + signed_report.len());
        instruction_data.extend_from_slice(&discriminator::VERIFY);

        let params = VerifyParams { signed_report };
        let param_data = params.try_to_vec().unwrap();
        instruction_data.extend_from_slice(&param_data);

        Instruction {
            program_id: *program_id,
            accounts,
            data: instruction_data,
        }
    }

    /// Helper to compute the verifier config PDA account.
    pub fn get_verifier_config_pda(program_id: &Pubkey) -> Pubkey {
        Pubkey::find_program_address(&[b"verifier"], program_id).0
    }

    /// Helper to compute the report config PDA account. This uses the first 32 bytes of the
    /// raw uncompressed report as the seed. This is validated within the verifier program
    pub fn get_config_pda(report: &[u8], program_id: &Pubkey) -> Pubkey {
        Pubkey::find_program_address(&[&report[..32]], program_id).0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_verify_instruction() {
        let program_id = Pubkey::new_unique();
        let verifier = Pubkey::new_unique();
        let controller = Pubkey::new_unique();
        let user = Pubkey::new_unique();
        let report = vec![1u8; 64];

        // Calculate expected PDA before moving report
        let expected_config = VerifierInstructions::get_config_pda(&report, &program_id);

        let ix = VerifierInstructions::verify(
            &program_id,
            &verifier,
            &controller,
            &user,
            &expected_config,
            report,
        );

        assert!(ix.data.starts_with(&discriminator::VERIFY));
        assert_eq!(ix.program_id, program_id);
        assert_eq!(ix.accounts.len(), 4);
        assert!(ix.accounts[2].is_signer);
        assert_eq!(ix.accounts[3].pubkey, expected_config);
    }
}
