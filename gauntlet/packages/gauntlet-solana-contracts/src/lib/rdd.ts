import { readFileSync } from 'fs'
import { join } from 'path'

const RDD_DIR = '../../reference-data-directory'
const DEFAULT_NETWORK = 'mainnet'

function load(network: string, path: string) {
  let buffer: any
  if (!path) {
    const newPath = network
      ? `${RDD_DIR}/directory-solana-${network}.json`
      : `${RDD_DIR}/directory-solana-${DEFAULT_NETWORK}.json`
    buffer = readFileSync(join(process.cwd(), newPath), 'utf8')
  }
  buffer = readFileSync(join(process.cwd(), path), 'utf8')
  try {
    const rdd = JSON.parse(buffer.toString())
    return rdd
  } catch (e) {
    throw new Error('An error ocurred while parsing the RDD. Make sure you provided a valid RDD path')
  }
}

function loadAggregator(network: string, rddPath: string, contractAddress: string) {
  const rdd = RDD.load(network, rddPath)
  const aggregator = rdd['contracts'][contractAddress]
  if (!aggregator) throw new Error(`Could not load aggregator: ${contractAddress}`)
  return aggregator
}

const RDD = {
  load,
  loadAggregator,
}
export default RDD
