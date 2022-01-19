import { ProgramError, parseIdlErrors, Idl } from '@project-serum/anchor'
import { Transaction, TransactionInstruction } from '@solana/web3.js'
import { RawTransaction, SolanaCommand } from '@chainlink/gauntlet-solana'

export const divideIntoChunks = (arr: Array<any> | Buffer, chunkSize: number): any[][] => {
  const chunks: any[] = []
  let prevIndex = 0
  while (prevIndex < arr.length) {
    chunks.push(arr.slice(prevIndex, prevIndex + chunkSize))
    prevIndex += chunkSize
  }
  return chunks
}

export const parseContractErrors = async (sendTx: Promise<String>, idl: Idl): Promise<string> => {
  let txhash
  try {
    txhash = await sendTx
  } catch (err) {
    // Translate IDL error
    const idlErrors = parseIdlErrors(idl)
    let translatedErr = ProgramError.parse(err, idlErrors)
    if (translatedErr === null) {
      throw err
    }
    throw translatedErr
  }
  return txhash
}

export const makeTx = async (rawTx: RawTransaction[]): Promise<Transaction> => {
  const tx = rawTx.reduce(
    (tx, meta) =>
      tx.add(
        new TransactionInstruction({
          programId: meta.programId,
          keys: meta.accounts,
          data: meta.data,
        }),
      ),
    new Transaction(),
  )
  return tx
}
