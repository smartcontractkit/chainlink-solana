import { Transaction, TransactionInstruction, TransactionBlockhashCtor, ComputeBudgetProgram } from '@solana/web3.js'

export const makeTx = (
  rawTx: TransactionInstruction[],
  opts?: TransactionBlockhashCtor,
  overrides: { price?: number; units?: number } = {},
): Transaction => {
  let initialTx = new Transaction(opts)

  if (overrides.price) initialTx.add(ComputeBudgetProgram.setComputeUnitPrice({ microLamports: overrides.price }))
  if (overrides.units) initialTx.add(ComputeBudgetProgram.setComputeUnitLimit({ units: overrides.units }))

  return rawTx.reduce((tx, instruction) => tx.add(instruction), initialTx)
}
