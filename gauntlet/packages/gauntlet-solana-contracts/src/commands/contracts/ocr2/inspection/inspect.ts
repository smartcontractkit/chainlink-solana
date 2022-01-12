import { Result } from '@chainlink/gauntlet-core'
import { inspection, BN, logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { Proto } from '@chainlink/gauntlet-core/dist/crypto'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey, SystemProgram } from '@solana/web3.js'

import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { getRDD } from '../../../../lib/rdd'
import WriteOffchainConfig, { Input as OffchainConfigInput } from '../offchainConfig/write'
import { descriptor as OCR2Descriptor } from '../../../../lib/ocr2Proto'
import { toComparableLongNumber, toComparableNumber, toComparablePubKey } from '../../../../lib/inspection'

type Input = {
  description: string
  decimals: string | number
  minAnswer: string | number
  maxAnswer: string | number
  transmitters: string[]
  offchainConfig: OffchainConfigInput
  billingAccessController: string
  requesterAccessController: string
  link: string
}

export default class OCR2Inspect extends SolanaCommand {
  static id = 'ocr2:inspect'
  static category = CONTRACT_LIST.OCR_2

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const info = rdd.contracts[this.flags.state]
    const aggregatorOperators: string[] = info.oracles.map((o) => o.operator)
    const transmitters = aggregatorOperators.map((operator) => rdd.operators[operator].ocrNodeAddress[0])
    const billingAccessController = this.flags.billingAccessController || process.env.BILLING_ACCESS_CONTROLLER
    const requesterAccessController = this.flags.requesterAccessController || process.env.REQUESTER_ACCESS_CONTROLLER
    const link = this.flags.link || process.env.LINK
    const offchainConfig = WriteOffchainConfig.makeInputFromRDD(rdd, this.flags.state)
    return {
      description: info.name,
      decimals: info.decimals,
      minAnswer: info.minSubmissionValue,
      maxAnswer: info.maxSubmissionValue,
      transmitters,
      billingAccessController,
      requesterAccessController,
      link,
      offchainConfig,
    }
  }

  constructor(flags, args) {
    super(flags, args)
    this.require(!!this.flags.state, 'Please provide flags with "state""')
  }

  deserializeConfig = (buffer: Buffer): any => {
    const proto = new Proto.Protobuf({ descriptor: OCR2Descriptor })
    const offchain = proto.decode('offchainreporting2_config.OffchainConfigProto', buffer)
    const reportingPluginConfig = proto.decode(
      'offchainreporting2_config.ReportingPluginConfig',
      offchain.reportingPluginConfig,
    )
    return { ...offchain, reportingPluginConfig }
  }

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

    const input = this.makeInput(this.flags.input)

    const state = new PublicKey(this.flags.state)
    const onChainState = await ocr2program.account.state.fetch(state)

    const bufferedConfig = Buffer.from(onChainState.config.offchainConfig.xs).slice(
      0,
      new BN(onChainState.config.offchainConfig.len).toNumber(),
    )

    const onChainOCRConfig = this.deserializeConfig(bufferedConfig)
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
