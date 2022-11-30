import {
  VersionedTransaction,
  TransactionMessage,
  ComputeBudgetProgram,
  TransactionMessageArgs,
  TransactionInstruction,
  Transaction,
} from '@solana/web3.js'

export const makeTx = (
  args: TransactionMessageArgs,
  overrides: { price?: number; units?: number } = {},
): VersionedTransaction => {
  if (overrides.price && overrides.units)
    throw new Error('Cannot set limit for units and price in the same transaction')

  let computeIx: TransactionInstruction
  if (overrides.price) computeIx = ComputeBudgetProgram.setComputeUnitPrice({ microLamports: overrides.price })
  if (overrides.units) computeIx = ComputeBudgetProgram.setComputeUnitLimit({ units: overrides.units })
  if (computeIx) {
    args.instructions.unshift(computeIx)
  }

  const messageV0 = new TransactionMessage(args).compileToV0Message()
  return new VersionedTransaction(messageV0)
}

// TODO: Remove once Ledger supports VersionedTransaction (https://github.com/LedgerHQ/app-solana/pull/48)
export const makeLegacyTx = (
  args: TransactionMessageArgs,
  overrides: { price?: number; units?: number } = {},
): Transaction => {
  if (overrides.price && overrides.units)
    throw new Error('Cannot set limit for units and price in the same transaction')

  let computeIx: TransactionInstruction
  if (overrides.price) computeIx = ComputeBudgetProgram.setComputeUnitPrice({ microLamports: overrides.price })
  if (overrides.units) computeIx = ComputeBudgetProgram.setComputeUnitLimit({ units: overrides.units })
  if (computeIx) {
    args.instructions.unshift(computeIx)
  }

  let tx = args.instructions.reduce((tx, ix) => tx.add(ix), new Transaction())
  return tx
}
