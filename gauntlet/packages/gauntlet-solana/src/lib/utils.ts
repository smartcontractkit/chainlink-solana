import bs58 from 'bs58'

import { Transaction, TransactionInstruction, TransactionCtorFields } from '@solana/web3.js'

export const makeTx = (rawTx: TransactionInstruction[], opts?: TransactionCtorFields): Transaction => {
  return rawTx.reduce((tx, instruction) => tx.add(instruction), new Transaction(opts))
}

export const validateAddress = (address: string): boolean => {
  const decoded = bs58.decode(address)
  if (!decoded || decoded.length != 32) {
    throw Error(`Invalid Solana address ${address}`)
    return false // not reached
  }
  return true
}
