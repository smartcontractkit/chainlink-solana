import {
  VersionedTransaction,
  TransactionMessage,
  ComputeBudgetProgram,
  TransactionMessageArgs,
  TransactionInstruction,
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
