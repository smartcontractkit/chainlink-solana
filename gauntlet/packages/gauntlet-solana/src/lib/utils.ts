import { Transaction, TransactionInstruction, TransactionCtorFields } from '@solana/web3.js'

export const makeTx = (rawTx: TransactionInstruction[], opts?: TransactionCtorFields): Transaction => {
  return rawTx.reduce((tx, instruction) => tx.add(instruction), new Transaction(opts))
}
