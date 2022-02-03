import { Transaction, TransactionCtorFields, TransactionInstruction } from '@solana/web3.js'
import { RawTransaction } from '@chainlink/gauntlet-solana'
import * as BufferLayout from '@solana/buffer-layout'

export const divideIntoChunks = (arr: Array<any> | Buffer, chunkSize: number): any[][] => {
  const chunks: any[] = []
  let prevIndex = 0
  while (prevIndex < arr.length) {
    chunks.push(arr.slice(prevIndex, prevIndex + chunkSize))
    prevIndex += chunkSize
  }
  return chunks
}

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

// Source: https://github.com/neonlabsorg/multisig/blob/8f1938c82c8db1251fad48a403487af18ecf5eb0/client/loader.ts#L25
export const encodeInstruction = (data: any): Buffer => {
  const CHUNK_SIZE = 900

  const dataLayout = BufferLayout.union(BufferLayout.u32('tag'), null, 'tag')
  dataLayout.addVariant(0, BufferLayout.struct([]), 'InitializeBuffer')
  const write = BufferLayout.struct([
    BufferLayout.u32('offset'),
    BufferLayout.nu64('length'),
    BufferLayout.seq(BufferLayout.u8('byte'), BufferLayout.offset(BufferLayout.u32(), -8), 'bytes'),
  ])
  dataLayout.addVariant(1, write, 'Write')
  const deployWithMaxLen = BufferLayout.struct([BufferLayout.nu64('max_data_len')])
  dataLayout.addVariant(2, deployWithMaxLen, 'DeployWithMaxDataLen')
  dataLayout.addVariant(3, BufferLayout.struct([]), 'Upgrade')
  dataLayout.addVariant(4, BufferLayout.struct([]), 'SetAuthority')
  dataLayout.addVariant(5, BufferLayout.struct([]), 'Close')

  // UpgradeableLoaderInstruction tag + offset + chunk length + chunk data
  const instructionBuffer = Buffer.alloc(4 + 4 + 8 + CHUNK_SIZE)
  const encodedSize = dataLayout.encode(data, instructionBuffer)
  return instructionBuffer.slice(0, encodedSize)
}
