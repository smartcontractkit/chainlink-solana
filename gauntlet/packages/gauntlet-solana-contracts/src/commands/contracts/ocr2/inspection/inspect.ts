import { Result } from '@chainlink/gauntlet-core'
import { inspection, BN, logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey, SystemProgram } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { deserializeConfig } from '../../../../lib/encoding'
import WriteOffchainConfig, { OffchainConfig } from '../proposeOffchainConfig'
import { toComparableLongNumber, toComparableNumber, toComparablePubKey } from '../../../../lib/inspection'
import RDD from '../../../../lib/rdd'

type Input = {
  description: string
  decimals: string | number
  minAnswer: string | number
  maxAnswer: string | number
  transmitters: string[]
  payees: string[]
  signers: string[]
  offchainConfig: OffchainConfig
  billingAccessController: string
  requesterAccessController: string
  billing: {
    observationPaymentGjuels: string
    transmissionPaymentGjuels: string
  }
}

export default class OCR2Inspect extends SolanaCommand {
  static id = 'ocr2:inspect'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:inspect --network=devnet --rdd=[PATH_TO_RDD] [AGGREGATOR_ADDRESS]',
    'yarn gauntlet ocr2:inspect [AGGREGATOR_ADDRESS]',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''
    const billingAccessController = this.flags.billingAccessController || process.env.BILLING_ACCESS_CONTROLLER
    const requesterAccessController = this.flags.requesterAccessController || process.env.REQUESTER_ACCESS_CONTROLLER

    // Return empty input if no rdd or user input provided
    if (!rddPath) {
      return {
        description: '',
        decimals: '',
        minAnswer: '',
        maxAnswer: '',
        transmitters: [],
        payees: [],
        signers: [],
        billingAccessController: '',
        requesterAccessController: '',
        offchainConfig: WriteOffchainConfig.makeInputFromRDD(null, this.args[0]),
        billing: {
          observationPaymentGjuels: '',
          transmissionPaymentGjuels: '',
        },
      }
    }

    const rdd = RDD.load(network, rddPath)
    const aggregator = RDD.loadAggregator(this.args[0], network, rddPath)
    const aggregatorOperators: string[] = aggregator.oracles.map((o) => o.operator)
    const transmitters = aggregatorOperators.map((operator) => rdd.operators[operator].ocrNodeAddress[0])
    const payees = aggregatorOperators.map((operator) => rdd.operators[operator].adminAddress)
    const signers = aggregatorOperators.map((operator) => rdd.operators[operator].ocr2OnchainPublicKey[0].substring(14))
    const offchainConfig = WriteOffchainConfig.makeInputFromRDD(rdd, this.args[0])

    return {
      description: aggregator.name,
      decimals: aggregator.decimals,
      minAnswer: aggregator.minSubmissionValue,
      maxAnswer: aggregator.maxSubmissionValue,
      transmitters,
      payees,
      signers,
      billingAccessController,
      requesterAccessController,
      offchainConfig,
      billing: {
        observationPaymentGjuels: aggregator.billing.observationPaymentGjuels,
        transmissionPaymentGjuels: aggregator.billing.transmissionPaymentGjuels,
      },
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  // toHexString converts a list of numbers to a hex string
  toHexString = (n: number[]) => Buffer.from(n).toString('hex')

  makeFeedInspections = async (bufferedInfo: Keypair, input: Input): Promise<inspection.Inspection[]> => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const storeProgram = this.loadProgram(store.idl, store.programId.toString())
    const account = await storeProgram.account.description.fetch(bufferedInfo.publicKey)
    return [
      inspection.makeInspection(
        // Description comes with some empty bytes
        Buffer.from(account.description.filter((v) => v !== 0)).toString(),
        input.description,
        'Description',
      ),
      inspection.makeInspection(
        toComparableNumber(account.config.decimals),
        toComparableNumber(input.decimals),
        'Decimals',
      ),
    ]
  }

  getTranmissionsStateAccount = async (tranmissions: PublicKey): Promise<Keypair> => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const storeProgram = this.loadProgram(store.idl, store.programId.toString())

    let buffer = Keypair.generate()
    await storeProgram.rpc.query(
      { version: {} },
      {
        accounts: {
          feed: tranmissions,
          buffer: buffer.publicKey,
        },
        preInstructions: [
          SystemProgram.createAccount({
            fromPubkey: this.provider.wallet.publicKey,
            newAccountPubkey: buffer.publicKey,
            lamports: await this.provider.connection.getMinimumBalanceForRentExemption(256),
            space: 256,
            programId: store.programId,
          }),
        ],
        signers: [buffer],
      },
    )

    return buffer
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const ocr2program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.args[0])
    const onChainState = await ocr2program.account.state.fetch(state)

    const bufferedConfig = Buffer.from(onChainState.offchainConfig.xs).slice(
      0,
      new BN(onChainState.offchainConfig.len).toNumber(),
    )

    const onChainOCRConfig = deserializeConfig(bufferedConfig)

    // Print on-chain oracle information
    onChainState.oracles.xs.forEach((oracle) => {
      logger.info(
        `Oracle Info:
          - Transmitter: ${oracle.transmitter}
          - Proposed Payee: ${oracle.proposedPayee}
          - From Round ID: ${oracle.fromRoundId}
          - Payment Gjuels: ${oracle.paymentGjuels}
      `,
      )
    })

    const input = this.makeInput(this.flags.input)
    // If input does not exist, just print config
    if (!input.description) {
      logger.info(`Min Answer: ${onChainState.config.minAnswer}`)
      logger.info(`Max Answer: ${onChainState.config.maxAnswer}`)
      logger.info(`Transmission Payment: ${onChainState.config.billing.transmissionPaymentGjuels}`)
      logger.info(`Observation Payment: ${onChainState.config.billing.observationPaymentGjuels}`)
      logger.info(`Requester Access Controller: ${onChainState.config.requesterAccessController}`)
      logger.info(`Billing Access Controller: ${onChainState.config.billingAccessController}`)
      return {
        responses: [
          {
            contract: state.toString(),
          },
        ],
      } as Result<TransactionResponse>
    }

    const wrappedComparableLongNumber = (v: any) => {
      // Proto encoding will ignore falsy values.
      if (!v) return '0'
      return toComparableLongNumber(v)
    }
    const longNumberInspections = [
      'deltaProgressNanoseconds',
      'deltaResendNanoseconds',
      'deltaRoundNanoseconds',
      'deltaGraceNanoseconds',
      'deltaStageNanoseconds',
      'maxDurationQueryNanoseconds',
      'maxDurationObservationNanoseconds',
      'maxDurationReportNanoseconds',
      'maxDurationShouldAcceptFinalizedReportNanoseconds',
      'maxDurationShouldTransmitAcceptedReportNanoseconds',
    ].map((prop) =>
      inspection.makeInspection(
        wrappedComparableLongNumber(onChainOCRConfig[prop]),
        toComparableNumber(input.offchainConfig[prop]),
        `Offchain Config "${prop}"`,
      ),
    )

    const inspections: inspection.Inspection[] = [
      inspection.makeInspection(
        toComparableNumber(onChainState.config.minAnswer),
        toComparableNumber(input.minAnswer),
        'Min Answer',
      ),
      inspection.makeInspection(
        toComparableNumber(onChainState.config.maxAnswer),
        toComparableNumber(input.maxAnswer),
        'Max Answer',
      ),
      inspection.makeInspection(
        toComparableNumber(onChainState.config.billing.transmissionPaymentGjuels),
        toComparableNumber(input.billing.transmissionPaymentGjuels),
        'Transmission Payment',
      ),
      inspection.makeInspection(
        toComparableNumber(onChainState.config.billing.observationPaymentGjuels),
        toComparableNumber(input.billing.observationPaymentGjuels),
        'Observation Payment',
      ),
      inspection.makeInspection(
        toComparablePubKey(onChainState.config.requesterAccessController),
        toComparablePubKey(input.requesterAccessController),
        'Requester access controller',
      ),
      inspection.makeInspection(
        toComparablePubKey(onChainState.config.billingAccessController),
        toComparablePubKey(input.billingAccessController),
        'Billing access controller',
      ),
      inspection.makeInspection(
        onChainState.oracles.xs
          .slice(0, onChainState.oracles.len)
          .map(({ transmitter }) => toComparablePubKey(transmitter)),
        input.transmitters.map(toComparablePubKey),
        'Transmitters',
      ),
      inspection.makeInspection(
        onChainState.oracles.xs.slice(0, onChainState.oracles.len).map(({ payee }) => toComparablePubKey(payee)),
        input.payees.map(toComparablePubKey),
        'Payees',
      ),
      inspection.makeInspection(
        onChainState.oracles.xs.slice(0, onChainState.oracles.len).map(({ signer }) => this.toHexString(signer.key)),
        input.signers,
        'Signers',
      ),
      // Offchain config inspection
      inspection.makeInspection(onChainOCRConfig.s, input.offchainConfig.s, 'Offchain Config "s"'),
      inspection.makeInspection(onChainOCRConfig.peerIds, input.offchainConfig.peerIds, 'Offchain Config "peerIds"'),
      inspection.makeInspection(
        toComparableNumber(onChainOCRConfig.rMax),
        toComparableNumber(input.offchainConfig.rMax),
        'Offchain Config "rMax"',
      ),
      ...longNumberInspections,
      inspection.makeInspection(
        onChainOCRConfig.reportingPluginConfig.alphaReportInfinite,
        input.offchainConfig.reportingPluginConfig.alphaReportInfinite,
        'Offchain Config "reportingPluginConfig.alphaReportInfinite"',
      ),
      inspection.makeInspection(
        onChainOCRConfig.reportingPluginConfig.alphaAcceptInfinite,
        input.offchainConfig.reportingPluginConfig.alphaAcceptInfinite,
        'Offchain Config "reportingPluginConfig.alphaAcceptInfinite"',
      ),
      inspection.makeInspection(
        wrappedComparableLongNumber(onChainOCRConfig.reportingPluginConfig.alphaReportPpb),
        toComparableNumber(input.offchainConfig.reportingPluginConfig.alphaReportPpb),
        `Offchain Config "reportingPluginConfig.alphaReportPpb"`,
      ),
      inspection.makeInspection(
        wrappedComparableLongNumber(onChainOCRConfig.reportingPluginConfig.alphaAcceptPpb),
        toComparableNumber(input.offchainConfig.reportingPluginConfig.alphaAcceptPpb),
        `Offchain Config "reportingPluginConfig.alphaAcceptPpb"`,
      ),
      inspection.makeInspection(
        wrappedComparableLongNumber(onChainOCRConfig.reportingPluginConfig.deltaCNanoseconds),
        toComparableNumber(input.offchainConfig.reportingPluginConfig.deltaCNanoseconds),
        `Offchain Config "reportingPluginConfig.deltaCNanoseconds"`,
      ),
    ]

    // Fetching tranmissions involves a tx. Give the option to the user to choose whether to fetch it or not.
    // Deactivated until we find a more efficient way to fetch this info
    // const withTransmissionsInfo = !!this.flags.withTransmissions
    // if (withTransmissionsInfo) {
    //   prompt('Fetching transmissions information involves sending a transaction. Continue?')
    //   const trasmissions = new PublicKey(onChainState.transmissions)
    //   const accountInfo = await this.getTranmissionsStateAccount(trasmissions)
    //   const tranmissionsInspections = await this.makeFeedInspections(accountInfo, input)
    //   inspections.push(...tranmissionsInspections)
    // }

    const isSuccessfulInspection = inspection.inspect(inspections)

    return {
      responses: [
        {
          tx: this.wrapInspectResponse(isSuccessfulInspection, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
