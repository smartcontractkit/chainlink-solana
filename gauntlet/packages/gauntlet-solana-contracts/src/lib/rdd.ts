import { readFileSync, writeFileSync } from 'fs'
import { join } from 'path'

const RDD_DIR = '../../reference-data-directory'
const DEFAULT_NETWORK = 'mainnet'

export enum CONTRACT_TYPES {
  PROXY = 'proxies',
  FLAG = 'flags',
  ACCESS_CONTROLLER = 'accessControllers',
  CONTRACT = 'contracts',
  VALIDATOR = 'validators',
}

function load(network = DEFAULT_NETWORK, path = `${RDD_DIR}/directory-solana-${network}.json`) {
  try {
    const buffer = readFileSync(path, 'utf8')
    const rdd = JSON.parse(buffer.toString())
    return rdd
  } catch (e) {
    throw new Error('An error ocurred while parsing the RDD. Make sure you provided a valid RDD path')
  }
}

function write(network = DEFAULT_NETWORK, path = `${RDD_DIR}/directory-solana-${network}.json`, rdd: any) {
  try {
    writeFileSync(path, JSON.stringify(rdd, null, 2))
    return
  } catch (e) {
    throw new Error('An error ocurred while parsing the RDD. Make sure you provided a valid RDD path')
  }
}

// TODO: repeated code - get rid of laodAggregator in favor for getContractFromRDD
function loadAggregator(contractAddress: string, network?: string, rddPath?: string) {
  if (!contractAddress) throw new Error('Could not fetch RDD without a valid aggregator address')
  const rdd = RDD.load(network, rddPath)
  const aggregator = rdd.contracts[contractAddress]
  if (!aggregator) throw new Error(`Could not load aggregator: ${contractAddress}`)
  return aggregator
}

function getContractFromRDD(rdd: any, address: string) {
  return Object.values(CONTRACT_TYPES).reduce((agg, type) => {
    const content = rdd[type]?.[address]
    if (content) {
      return {
        type,
        contract: content,
        address,
        ...((type === CONTRACT_TYPES.CONTRACT || type === CONTRACT_TYPES.PROXY) && { description: content.name }),
      }
    }
    return agg
  }, {} as any)
}

const RDD = {
  load,
  write,
  loadAggregator,
  getContractFromRDD,
}
export default RDD
