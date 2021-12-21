import { AccountMeta, PublicKey } from '@solana/web3.js'

export type TransactionResponse = {
  hash: string
  address?: string
  wait: (hash: string) => Promise<{ success: boolean }>
  tx?: any
  states?: Record<string, string>
}

export type RawTransaction = {
  data: Buffer
  accounts: AccountMeta[]
  programId: PublicKey
}
