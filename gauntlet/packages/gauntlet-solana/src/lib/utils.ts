import { Transaction, TransactionInstruction, TransactionBlockhashCtor, ComputeBudgetProgram } from '@solana/web3.js'

export interface Overrides {
  price?: number
  units?: number
}

export const makeTx = (
  rawTx: TransactionInstruction[],
  opts?: TransactionBlockhashCtor,
  overrides: Overrides = {},
): Transaction => {
  let initialTx = new Transaction(opts)

  if (overrides.price) initialTx.add(ComputeBudgetProgram.setComputeUnitPrice({ microLamports: overrides.price }))
  if (overrides.units) initialTx.add(ComputeBudgetProgram.setComputeUnitLimit({ units: overrides.units }))
  return rawTx.reduce((tx, instruction) => tx.add(instruction), initialTx)
}

export const median = (values: number[]): number | null => {
  if (values.length === 0) return null // Handle empty arrays

  // Sort the array in ascending order
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)

  // If even, return the average of the two middle numbers
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid]
}
