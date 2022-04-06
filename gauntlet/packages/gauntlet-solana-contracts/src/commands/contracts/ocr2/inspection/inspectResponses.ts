import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { deserializeConfig } from '../../../../lib/encoding'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { inspection, logger, BN } from '@chainlink/gauntlet-core/dist/utils'

type Input = {}

export default class OCR2InspectResponses extends SolanaCommand {
  static id = 'ocr2:inspect:responses'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:inspect:responses --network=devnet --rdd=[PATH_TO_RDD] [AGGREGATOR_ADDRESS]',
    'yarn gauntlet ocr2:inspect:responses [AGGREGATOR_ADDRESS]',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input

    return {}
  }

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.args[0])
    const input = this.makeInput(this.flags.input)
    const onChainState = await program.account.state.fetch(state)

    logger.info(`Latest Transmission:
  - Latest Transmitter: ${onChainState.config.latestTransmitter}
  - Latest Aggregator Round ID: ${onChainState.config.latestAggregatorRoundId}
  - Latest Config Digest: ${onChainState.config.latestConfigDigest}
  - Latest Config Block Number: ${onChainState.config.latestConfigBlockNumber}`)

  /*
    const bufferedConfig = Buffer.from(onChainState.offchainConfig.xs).slice(
      0,
      new BN(onChainState.offchainConfig.len).toNumber(),
    )
    const onChainOCRConfig = deserializeConfig(bufferedConfig)
    console.log(onChainOCRConfig)
    */

    const inspections: inspection.Inspection[] = []

    const successfulInspection = inspection.inspect(inspections)

    return {
      responses: [
        {
          tx: this.wrapInspectResponse(successfulInspection, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
