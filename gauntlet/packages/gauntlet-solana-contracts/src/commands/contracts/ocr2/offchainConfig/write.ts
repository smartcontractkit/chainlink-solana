import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { MAX_TRANSACTION_BYTES } from '../../../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { Protobuf } from '../../../../core/proto'
import { descriptor as OCR2Descriptor } from '../../../../core/proto/ocr2Proto'
import { getRDD } from '../../../../lib/rdd'
import { makeSharedSecretEncryptions, SharedSecretEncryptions } from '../../../../core/sharedSecretEncryptions'
import { durationToNanoseconds } from '../../../../core/time'
import { divideIntoChunks } from '../../../../core/utils'

type Input = {
  deltaProgressNanoseconds: number
  deltaResendNanoseconds: number
  deltaRoundNanoseconds: number
  deltaGraceNanoseconds: number
  deltaStageNanoseconds: number
  rMax: number
  s: number[]
  offchainPublicKeys: string[]
  peerIds: string[]
  reportingPluginConfig: {
    alphaReportInfinite: boolean
    alphaReportPpb: number
    alphaAcceptInfinite: boolean
    alphaAcceptPpb: number
    deltaCNanoseconds: number
  }
  maxDurationQueryNanoseconds: number
  maxDurationObservationNanoseconds: number
  maxDurationReportNanoseconds: number
  maxDurationShouldAcceptFinalizedReportNanoseconds: number
  maxDurationShouldTransmitAcceptedReportNanoseconds: number
  configPublicKeys: string[]
}

export default class WriteOffchainConfig extends SolanaCommand {
  static id = 'ocr2:write_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:write_offchain_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
    this.require(
      !!process.env.SECRET,
      'Please specify the Gauntlet secret words e.g. SECRET="awe fluke polygon tonic lilly acuity onyx debra bound gilbert wane"',
    )
  }

  makeInput = (userInput: any): Input => {
    // TODO: Some format validation for user input
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const aggregator = rdd.contracts[this.flags.state]
    const config = aggregator.config
    const aggregatorOperators: string[] = aggregator.oracles.map((o) => o.operator)
    const operatorsPublicKeys = aggregatorOperators.map((o) => rdd.operators[o].ocrOffchainPublicKey[0])
    const operatorsPeerIds = aggregatorOperators.map((o) => rdd.operators[o].peerId[0])
    const operatorConfigPublicKeys = aggregatorOperators.map((o) => rdd.operators[o].ocrConfigPublicKey[0])

    const input: Input = {
      deltaProgressNanoseconds: durationToNanoseconds(config.deltaProgress).toNumber(),
      deltaResendNanoseconds: durationToNanoseconds(config.deltaResend).toNumber(),
      deltaRoundNanoseconds: durationToNanoseconds(config.deltaRound).toNumber(),
      deltaGraceNanoseconds: durationToNanoseconds(config.deltaGrace).toNumber(),
      deltaStageNanoseconds: durationToNanoseconds(config.deltaStage).toNumber(),
      rMax: config.rMax,
      s: config.s,
      offchainPublicKeys: operatorsPublicKeys,
      peerIds: operatorsPeerIds,
      reportingPluginConfig: {
        alphaReportInfinite: config.reportingPluginConfig.alphaReportInfinite,
        alphaReportPpb: Number(config.reportingPluginConfig.alphaReportPpb),
        alphaAcceptInfinite: config.reportingPluginConfig.alphaAcceptInfinite,
        alphaAcceptPpb: Number(config.reportingPluginConfig.alphaAcceptPpb),
        deltaCNanoseconds: durationToNanoseconds(config.reportingPluginConfig.deltaC).toNumber(),
      },
      maxDurationQueryNanoseconds: durationToNanoseconds(config.maxDurationQuery).toNumber(),
      maxDurationObservationNanoseconds: durationToNanoseconds(config.maxDurationObservation).toNumber(),
      maxDurationReportNanoseconds: durationToNanoseconds(config.maxDurationReport).toNumber(),
      maxDurationShouldAcceptFinalizedReportNanoseconds: durationToNanoseconds(
        config.maxDurationShouldAcceptFinalizedReport,
      ).toNumber(),
      maxDurationShouldTransmitAcceptedReportNanoseconds: durationToNanoseconds(
        config.maxDurationShouldTransmitAcceptedReport,
      ).toNumber(),
      configPublicKeys: operatorConfigPublicKeys,
    }
    return input
  }

  serializeOffchainConfig = async (input: Input): Promise<Buffer> => {
    const proto = new Protobuf({ descriptor: OCR2Descriptor })
    const reportingPluginConfigProto = proto.encode(
      'offchainreporting2_config.ReportingPluginConfig',
      input.reportingPluginConfig,
    )
    const sharedSecretEncryptions = await this.generateSecretEncryptions(input.configPublicKeys)
    const offchainConfig = {
      ...input,
      reportingPluginConfig: reportingPluginConfigProto,
      sharedSecretEncryptions,
    }
    return Buffer.from(proto.encode('offchainreporting2_config.OffchainConfigProto', offchainConfig))
  }

  // constructs a SharedSecretEncryptions from
  // a set of SharedSecretEncryptionPublicKeys, the sharedSecret, and a cryptographic randomness source
  generateSecretEncryptions = async (operatorsPublicKeys: string[]): Promise<SharedSecretEncryptions> => {
    const gauntletSecret = process.env.SECRET
    return makeSharedSecretEncryptions(gauntletSecret!, operatorsPublicKeys)
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const owner = this.wallet.payer

    const input = this.makeInput(this.flags.input)
    const offchainConfig = await this.serializeOffchainConfig(input)

    logger.info(`Offchain config size: ${offchainConfig.byteLength}`)
    this.require(offchainConfig.byteLength < 4096, 'Offchain config must be lower than 4096 bytes')

    // There's a byte limit per transaction. Write the config in chunks
    const offchainConfigChunks = divideIntoChunks(offchainConfig, MAX_TRANSACTION_BYTES)
    if (offchainConfigChunks.length > 1) {
      logger.info(
        `Config size (${offchainConfig.byteLength} bytes) is bigger than transaction limit. It will be configured using ${offchainConfigChunks.length} transactions`,
      )
    }

    await prompt('Start writing offchain config?')

    const txs: string[] = []
    for (let i = 0; i < offchainConfigChunks.length; i++) {
      logger.loading(`Sending ${i + 1}/${offchainConfigChunks.length}...`)
      const tx = await program.rpc.writeOffchainConfig(offchainConfigChunks[i], {
        accounts: {
          state: state,
          authority: owner.publicKey,
        },
      })
      txs.push(tx)
    }
    logger.success(`Last tx Write offchain config set on tx ${txs[txs.length - 1]}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txs[txs.length - 1], state.toString(), { state: state.toString() }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
