import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { readFileSync } from 'fs'
import { join } from 'path'
import { MAX_TRANSACTION_BYTES } from '../../../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { Protobuf } from '../../../../lib/protobuf'
import { offchainDescriptor } from '../../../../lib/protoSchemas'
import { generateSecretWords } from '../../../../lib/random'
import { makeSharedSecretEncryptions, SharedSecretEncryptions } from '../../../../lib/sharedSecretEncryptions'
import { durationToNanoseconds } from '../../../../lib/time'
import { divideIntoChunks } from '../../../../lib/utils'

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
    const buffer = readFileSync(join(process.cwd(), this.flags.rdd), 'utf8')
    try {
      const rdd = JSON.parse(buffer.toString())
      const aggregator = rdd.contracts[this.flags.state]
      const config = aggregator.config
      const aggregatorOperators: string[] = aggregator.oracles.map(o => o.operator)
      const operatorsPublicKeys = aggregatorOperators.map(o => rdd.operators[o].ocrOffchainPublicKey[0])
      const operatorsPeerIds = aggregatorOperators.map(o => rdd.operators[o].peerId[0])

      const input: Input = {
        deltaProgressNanoseconds: durationToNanoseconds(config.badEpochTimeout).toNumber(),
        deltaResendNanoseconds: durationToNanoseconds(config.resendInterval).toNumber(),
        deltaRoundNanoseconds: durationToNanoseconds(config.roundInterval).toNumber(),
        deltaGraceNanoseconds: durationToNanoseconds(config.observationGracePeriod).toNumber(),
        deltaStageNanoseconds: durationToNanoseconds(config.transmissionStageTimeout).toNumber(),
        rMax: config.maxRoundCount,
        s: config.transmissionStages,
        offchainPublicKeys: operatorsPublicKeys,
        peerIds: operatorsPeerIds,
        reportingPluginConfig: {
          alphaReportInfinite: true, // bool
          alphaReportPpb: config.relativeDeviationThresholdPPB, // ??
          alphaAcceptInfinite: true, // bool
          alphaAcceptPpb: 1, // bool
          deltaCNanoseconds: durationToNanoseconds(config.maxContractValueAge).toNumber(),
        },
        maxDurationQueryNanoseconds: 1, // number (ns)
        maxDurationObservationNanoseconds: 1, // number (ns)
        maxDurationReportNanoseconds: 1, // number (ns)
        maxDurationShouldAcceptFinalizedReportNanoseconds: 1, // number (ns)
        maxDurationShouldTransmitAcceptedReportNanoseconds: 1, // number (ns)
      }
      return input
    } catch (e) {
      throw new Error('An error ocurred while parsing the RDD. Make sure you provided a valid RDD path')
    }
  }

  serializeOffchainConfig = async (input: Input): Promise<Buffer> => {
    const proto = new Protobuf(offchainDescriptor)
    const reportingPluginConfigProto = proto.encode('reporting_plugin_config', input.reportingPluginConfig)
    const sharedSecretEncryptions = await this.generateSecretEncryptions(input.offchainPublicKeys)
    const sharedSecretEncryptionsProto = proto.encode('shared_secret_encryptions', sharedSecretEncryptions)
    const offchainConfig = {
      ...input,
      reportingPluginConfig: reportingPluginConfigProto,
      sharedSecretEncryptions: sharedSecretEncryptionsProto,
    }
    return Buffer.from(proto.encode('offchain_config', offchainConfig))
  }

  // constructs a SharedSecretEncryptions from
  // a set of SharedSecretEncryptionPublicKeys, the sharedSecret, and a cryptographic randomness source
  generateSecretEncryptions = async (operatorsPublicKeys: string[]): Promise<SharedSecretEncryptions> => {
    const gauntletSecret = process.env.SECRET
    const signerSecret = await generateSecretWords()
    return makeSharedSecretEncryptions(gauntletSecret!, signerSecret, operatorsPublicKeys)
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const owner = this.wallet.payer

    const input = this.makeInput(this.flags.input)

    const offchainConfig = await this.serializeOffchainConfig(input)
    console.log('offchain config size:', offchainConfig.byteLength)
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
