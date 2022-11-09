import { Transaction, TransactionInstruction, TransactionBlockhashCtor, ComputeBudgetProgram } from '@solana/web3.js'

export const makeTx = (
  rawTx: TransactionInstruction[],
  opts?: TransactionBlockhashCtor,
  overrides: { price?: number; units?: number } = {},
): Transaction => {
  if (overrides.price && overrides.units)
    throw new Error('Cannot set limit for units and price in the same transaction')

  let computeIx: TransactionInstruction
  if (overrides.price) computeIx = ComputeBudgetProgram.setComputeUnitPrice({ microLamports: overrides.price })
  if (overrides.units) computeIx = ComputeBudgetProgram.setComputeUnitLimit({ units: overrides.units })
  const initialTx = computeIx ? new Transaction(opts).add(computeIx) : new Transaction(opts)

  return rawTx.reduce((tx, instruction) => tx.add(instruction), initialTx)
}
