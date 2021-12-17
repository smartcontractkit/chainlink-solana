import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { getRDD } from '../../../../lib/rdd'
import {
  inspect,
  Inspection,
  makeInspection,
  toComparableNumber,
  toComparablePubKey,
  toComparableLongNumber,
} from '../../../../core/inspection'
import WriteOffchainConfig, { Input as OffchainConfigInput } from '../offchainConfig/write'
import BN from 'bn.js'
import { Protobuf } from '../../../../core/proto'
import { descriptor as OCR2Descriptor } from '../../../../core/proto/ocr2Proto'

type Input = {
  description: string
  decimals: string | number
  minAnswer: string | number
  maxAnswer: string | number
  transmitters: string[]
  billingAccessController: string
  requesterAccessController: string
  link: string
  offchainConfig: OffchainConfigInput
}

export default class OCR2Inspect extends SolanaCommand {
  static id = 'ocr2:inspect'
  static category = CONTRACT_LIST.OCR_2

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const info = rdd.contracts[this.flags.state]
    const aggregatorOperators: string[] = info.oracles.map((o) => o.operator)
    const transmitters = aggregatorOperators.map((operator) => rdd.operators[operator].nodeAddress[0])
    const billingAccessController =
      this.flags.billingAccessController || process.env.BILLING_ADMIN_ACCESS_CONTROLLER_ADDRESS
    const requesterAccessController =
      this.flags.requesterAccessController || process.env.REQUESTER_ADMIN_ACCESS_CONTROLLER_ADDRESS
    const link = this.flags.link || process.env.LINK_ADDRESS
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
    const proto = new Protobuf({ descriptor: OCR2Descriptor })
    const offchain = proto.decode('offchainreporting2_config.OffchainConfigProto', buffer)
    const reportingPluginConfig = proto.decode(
      'offchainreporting2_config.ReportingPluginConfig',
      offchain.reportingPluginConfig,
    )
    return { ...offchain, reportingPluginConfig }
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.flags.state)
    const input = this.makeInput(this.flags.input)
    const onChainState = await program.account.state.fetch(state)

    const bufferedConfig = Buffer.from(onChainState.config.offchainConfig.xs).slice(
      0,
      new BN(onChainState.config.offchainConfig.len).toNumber(),
    )

    const onChainOCRConfig = this.deserializeConfig(bufferedConfig)
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
      makeInspection(
        toComparableLongNumber(onChainOCRConfig[prop]),
        toComparableNumber(input.offchainConfig[prop]),
        `Offchain Config "${prop}"`,
      ),
    )

    const inspections: Inspection[] = [
      makeInspection(
        toComparableNumber(onChainState.config.minAnswer),
        toComparableNumber(input.minAnswer),
        'Min Answer',
      ),
      makeInspection(
        toComparableNumber(onChainState.config.maxAnswer),
        toComparableNumber(input.maxAnswer),
        'Max Answer',
      ),
      makeInspection(toComparableNumber(onChainState.config.decimals), toComparableNumber(input.decimals), 'Decimals'),
      makeInspection(
        // Description comes with some empty bytes
        Buffer.from(onChainState.config.description.filter((v) => v !== 0)).toString(),
        input.description,
        'Description',
      ),
      makeInspection(
        toComparablePubKey(onChainState.config.requesterAccessController),
        toComparablePubKey(input.requesterAccessController),
        'Requester access controller',
      ),
      makeInspection(
        toComparablePubKey(onChainState.config.billingAccessController),
        toComparablePubKey(input.billingAccessController),
        'Requester access controller',
      ),
      makeInspection(
        onChainState.oracles.xs
          .slice(0, onChainState.oracles.len)
          .map(({ transmitter }) => toComparablePubKey(transmitter)),
        input.transmitters.map(toComparablePubKey),
        'Transmitters',
      ),
      // Offchain config inspection
      makeInspection(onChainOCRConfig.s, input.offchainConfig.s, 'Offchain Config "s"'),
      makeInspection(onChainOCRConfig.peerIds, input.offchainConfig.peerIds, 'Offchain Config "peerIds"'),
      makeInspection(
        toComparableNumber(onChainOCRConfig.rMax),
        toComparableNumber(input.offchainConfig.rMax),
        'Offchain Config "rMax"',
      ),
      ...longNumberInspections,
      makeInspection(
        onChainOCRConfig.reportingPluginConfig.alphaReportInfinite,
        input.offchainConfig.reportingPluginConfig.alphaReportInfinite,
        'Offchain Config "reportingPluginConfig.alphaReportInfinite"',
      ),
      makeInspection(
        onChainOCRConfig.reportingPluginConfig.alphaAcceptInfinite,
        input.offchainConfig.reportingPluginConfig.alphaAcceptInfinite,
        'Offchain Config "reportingPluginConfig.alphaAcceptInfinite"',
      ),
      makeInspection(
        toComparableLongNumber(onChainOCRConfig.reportingPluginConfig.alphaReportPpb),
        toComparableNumber(input.offchainConfig.reportingPluginConfig.alphaReportPpb),
        `Offchain Config "reportingPluginConfig.alphaReportPpb"`,
      ),
      makeInspection(
        toComparableLongNumber(onChainOCRConfig.reportingPluginConfig.alphaAcceptPpb),
        toComparableNumber(input.offchainConfig.reportingPluginConfig.alphaAcceptPpb),
        `Offchain Config "reportingPluginConfig.alphaAcceptPpb"`,
      ),
      makeInspection(
        toComparableLongNumber(onChainOCRConfig.reportingPluginConfig.deltaCNanoseconds),
        toComparableNumber(input.offchainConfig.reportingPluginConfig.deltaCNanoseconds),
        `Offchain Config "reportingPluginConfig.deltaCNanoseconds"`,
      ),
    ]

    const isSuccessfulInspection = inspect(inspections)

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
