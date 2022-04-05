import { Transaction, TransactionCtorFields, TransactionInstruction } from '@solana/web3.js'
import assert from 'assert'

export const isDeepEqual = (a: any, b: any) => {
  try {
    assert.deepStrictEqual(a, b)
  } catch (error) {
    if (error.name === 'AssertionError') {
      return false
    }
    throw error
  }
  return true
}

export const makeTx = (rawTx: TransactionInstruction[], opts?: TransactionCtorFields): Transaction => {
  return rawTx.reduce((tx, instruction) => tx.add(instruction), new Transaction(opts))
}
