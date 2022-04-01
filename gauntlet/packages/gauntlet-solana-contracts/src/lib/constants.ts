import { PublicKey } from '@solana/web3.js'

export const MAX_TRANSACTION_BYTES = 996
export const ORACLES_MAX_LENGTH = 19
export const UPGRADEABLE_BPF_LOADER_PROGRAM_ID = new PublicKey('BPFLoaderUpgradeab1e11111111111111111111111')
export const ADDITIONAL_STATE_BUFFER = 1024

// Solana uses u64 for token values. We couldn't fit the entire LINK supply with 18 decimals. Most tokens use 9 or even 8 decimals
export const TOKEN_DECIMALS = 9

// For multisig config
export const MAX_BUFFER_SIZE = 796
