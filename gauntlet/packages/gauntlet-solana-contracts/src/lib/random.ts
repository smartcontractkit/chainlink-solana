import * as readline from 'readline'
import * as fs from 'fs'
import { join } from 'path'

export async function generateSecretWords(sampleSize: number = 12): Promise<string> {
  if (sampleSize <= 0) {
    throw new RangeError('Requested 0 or less words')
  }

  const fileStream = fs.createReadStream(
    join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/words', 'words.txt'),
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

// Python's `.sample` ported over to TS: https://stackoverflow.com/a/45556840/8605244
function sample<T>(population: T[], k: number): T[] {
  const n = population.length

  if (k < 0 || k > n) throw new RangeError('Sample size larger than population or is negative')

  const result = new Array(k)
  let setsize = 21 // size of a small set minus size of an empty list
  if (k > 5) {
    setsize += Math.pow(4, Math.ceil(Math.log(k * 3) / Math.log(4)))
  }

  if (n <= setsize) {
    // An n-length list is smaller than a sampleSize-length set
    const pool = population.slice()
    for (let i = 0; i < k; i++) {
      // invariant:  non-selected at [0,n-i)
      const j = (Math.random() * (n - i)) | 0
      result[i] = pool[j]
      pool[j] = pool[n - i - 1] // move non-selected item into vacancy
    }
  } else {
    const selected = new Set()
    for (let i = 0; i < k; i++) {
      let j = (Math.random() * n) | 0
      while (selected.has(j)) {
        j = (Math.random() * n) | 0
      }
      selected.add(j)
      result[i] = population[j]
    }
  }

  return result
}
