import { Transaction, TransactionInstruction, Keypair } from '@solana/web3.js'
import { RawTransaction } from '@chainlink/gauntlet-solana'

export const divideIntoChunks = (arr: Array<any> | Buffer, chunkSize: number): any[][] => {
  const chunks: any[] = []
  let prevIndex = 0
  while (prevIndex < arr.length) {
    chunks.push(arr.slice(prevIndex, prevIndex + chunkSize))
    prevIndex += chunkSize
  }
  return chunks
}

export const makeTx = (rawTx: RawTransaction[]): Transaction => {
  return rawTx.reduce(
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
}
