import { Transaction, TransactionInstruction, TransactionCtorFields } from '@solana/web3.js'
import { RawTransaction } from '..'

export const makeTx = (rawTx: RawTransaction[], opts?: TransactionCtorFields): Transaction => {
  return rawTx.reduce(
    (tx, meta) =>
      tx.add(
        new TransactionInstruction({
          programId: meta.programId,
          keys: meta.accounts,
          data: meta.data,
        }),
      ),
    new Transaction(opts),
  )
}
