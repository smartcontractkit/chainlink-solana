import { SolanaCommand } from '@chainlink/solana-gauntlet'

interface Oracle {
  signer: Buffer
  transmitter: Buffer
}

export interface OCR2Config {
  oracles: Oracle[]
  threshold: number
  onchainConfig: Buffer
  offchainConfig: Buffer
  offchainConfigVersion: number
}

export type SolanaConstructor = new (flags, args) => SolanaCommand
