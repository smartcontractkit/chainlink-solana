import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { inspection, logger } from '@chainlink/gauntlet-core/dist/utils'
import { getLatestNewTransmissionEvents } from '../../../../lib/events'

type Input = {}

export default class OCR2InspectResponses extends SolanaCommand {
  static id = 'ocr2:inspect:responses'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:inspect:responses --network=devnet [AGGREGATOR_ADDRESS]']

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

    logger.info(`Latest Config: 
    - Latest Transmitter: ${onChainState.config.latestTransmitter}
    - Latest Aggregator Round ID: ${onChainState.config.latestAggregatorRoundId}
    - Latest Config Digest: ${onChainState.config.latestConfigDigest}
    - Latest Config Block Number: ${onChainState.config.latestConfigBlockNumber}
    `)

    const transmitters = onChainState.oracles.xs
      .map((oracle) => oracle.transmitter)
      .filter((transmitter) => transmitter._bn != 0)

    // TODO: Get the info we need. Match trasmitter index with our list, observers...
    const events = await getLatestNewTransmissionEvents(this.provider.connection, state, program)

    console.log(events)

    // await this.getLatestTransmissionEvent(state, program, transmitters, (event: NewTransmission) => {
    //   // Log transmission data
    //   logger.info(
    //     `Latest Transmission
    // - Round Id: ${event.roundId}
    // - Config Digest: ${event.configDigest}
    // - Answer: ${event.answer}
    // - Transmitter: ${event.transmitter}
    // - Observations Timestamp: ${event.observationsTimestamp}
    // - Observer Count: ${event.observerCount}
    // - Observers: ${event.observers}
    // - Juels Per Lamport: ${event.juelsPerLamport}
    // - Reimbursement Gjuels: ${event.reimbursementGjuels}
    //     `,
    //   )
    //   // Log responding oracle count
    //   logger.info(
    //     `${event.observerCount}/${transmitters.length} oracles are responding
    //     `,
    //   )
    //   // Log oracles that are not responsive
    //   transmitters.forEach((transmitter) => {
    //     // If the transmitter is not listed as an observer, log it
    //     if (!event.observers.includes(transmitter)) {
    //       logger.error(
    //         `Oracle ${transmitter} not responding
    //         `,
    //       )
    //     }
    //   })
    //   return
    // })

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
