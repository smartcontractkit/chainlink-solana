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
