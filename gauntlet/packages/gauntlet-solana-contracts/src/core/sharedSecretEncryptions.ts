import * as crypto from './crypto'
import * as readline from 'readline'
import * as fs from 'fs'
import { join } from 'path'
import { sample } from './random'

export type SharedSecretEncryptions = {
  diffieHellmanPoint: Buffer
  sharedSecretHash: Buffer
  encryptions: Buffer[]
}

export const makeSharedSecretEncryptions = async (
  x_signersWords: string,
  nodes: string[],
  x_proposerWords?: string,
): Promise<SharedSecretEncryptions> => {
  const x_signers = crypto.keccak256(Buffer.from(x_signersWords))
  if (!x_proposerWords) {
    x_proposerWords = await generateSecretWords()
  }
  const x_proposer = crypto.keccak256(Buffer.from(x_proposerWords))
  const x_oracles = crypto.compute_x_oracles(x_signers, x_proposer)
  const { publicKey, secretKey } = crypto.x25519_keypair(x_signers, x_proposer)
  console.log('nodes: ')
  console.log(nodes)

  const encryptions: Buffer[] = nodes.map((node) => {
    const pk_i = Buffer.from(node, 'hex')
    const key_i = crypto.compute_key_i(pk_i, secretKey)
    return crypto.compute_enc_i(key_i, x_oracles)
  })

  return {
    diffieHellmanPoint: publicKey,
    sharedSecretHash: crypto.keccak256(x_oracles),
    encryptions,
  }
}

export async function generateSecretWords(sampleSize: number = 12): Promise<string> {
  if (sampleSize <= 0) {
    throw new RangeError('Requested 0 or less words')
  }

  const fileStream = fs.createReadStream(
    join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/bip-0039', 'english.txt'),
  )
  const rl = readline.createInterface({
    input: fileStream,
    crlfDelay: Infinity,
  })
  const wordList: string[] = []
  for await (const line of rl) {
    const word = line.trim()
    if (word.length > 0) wordList.push(word)
  }

  const n = wordList.length
  if (n < sampleSize) {
    throw new RangeError(`Requested ${sampleSize} words, but wordlist only has ${n} words`)
  }

  return sample(wordList, sampleSize).join(' ')
}
