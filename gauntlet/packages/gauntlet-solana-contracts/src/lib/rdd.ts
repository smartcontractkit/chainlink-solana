import { readFileSync } from 'fs'
import { join } from 'path'

export const getRDD = (path: string) => {
  const buffer = readFileSync(join(process.cwd(), path), 'utf8')
  try {
    const rdd = JSON.parse(buffer.toString())
    return rdd
  } catch (e) {
    throw new Error('An error ocurred while parsing the RDD. Make sure you provided a valid RDD path')
  }
}
