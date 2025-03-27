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

export const percentile = (values: number[], p: number): number | null => {
  if (values.length === 0) return null // Handle empty arrays
  if (p < 0 || p > 1) throw new Error('Percentile must be between 0 and 1')

  // Sort the array in ascending order
  const sorted = [...values].sort((a, b) => a - b)

  // Compute the index based on the percentile
  const index = (sorted.length - 1) * p
  const lower = Math.floor(index)
  const upper = Math.ceil(index)

  // If index is an integer, return the value directly
  if (lower === upper) return sorted[lower]

  // Interpolate between the two closest values
  return sorted[lower] + (index - lower) * (sorted[upper] - sorted[lower])
}
