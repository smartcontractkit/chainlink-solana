import WriteOffchainConfig, { OffchainConfig } from '../proposeOffchainConfig'
import RDD from '../../../../lib/rdd'

export type Input = {
  description: string
  decimals: string | number
  minAnswer: string | number
  maxAnswer: string | number
  transmitters: string[]
  payees: string[]
  signers: string[]
  displayNames: string[]
  websites: string[]
  offchainConfig: OffchainConfig
  billingAccessController: string
  requesterAccessController: string
  billing: {
    observationPaymentGjuels: string
    transmissionPaymentGjuels: string
  }
}

export const makeInput = (flags, args): Input | undefined => {
  if (flags.input) return flags.input as Input
  const network = flags.network || ''
  const rddPath = flags.rdd || ''
  const billingAccessController = flags.billingAccessController || process.env.BILLING_ACCESS_CONTROLLER
  const requesterAccessController = flags.requesterAccessController || process.env.REQUESTER_ACCESS_CONTROLLER

  // Return empty input if no rdd or user input provided
  if (!rddPath) {
    return undefined
  }

  const rdd = RDD.load(network, rddPath)
  const aggregator = RDD.loadAggregator(args[0], network, rddPath)
  const aggregatorOperators: string[] = aggregator.oracles.map((o) => o.operator)
  const transmitters = aggregatorOperators.map((operator) => rdd.operators[operator].ocrNodeAddress[0])
  const payees = aggregatorOperators.map((operator) => rdd.operators[operator].adminAddress)
  const displayNames = aggregatorOperators.map((operator) => rdd.operators[operator].displayName)
  const websites = aggregatorOperators.map((operator) => rdd.operators[operator].website)
  const signers = aggregatorOperators.map((operator) => rdd.operators[operator].ocr2OnchainPublicKey[0].substring(14))
  const offchainConfig = WriteOffchainConfig.makeInputFromRDD(rdd, args[0])

  return {
    description: aggregator.name,
    decimals: aggregator.decimals,
    minAnswer: aggregator.minSubmissionValue,
    maxAnswer: aggregator.maxSubmissionValue,
    transmitters,
    payees,
    signers,
    displayNames,
    websites,
    billingAccessController,
    requesterAccessController,
    offchainConfig,
    billing: {
      observationPaymentGjuels: aggregator.billing.observationPaymentGjuels,
      transmissionPaymentGjuels: aggregator.billing.transmissionPaymentGjuels,
    },
  }
}
