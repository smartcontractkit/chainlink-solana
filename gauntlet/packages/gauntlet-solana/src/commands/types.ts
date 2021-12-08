export type TransactionResponse = {
  hash: string
  address?: string
  wait: (hash: string) => Promise<{ success: boolean }>
  tx?: any
  states?: Record<string, string>
}
