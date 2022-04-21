import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { inspection, logger } from '@chainlink/gauntlet-core/dist/utils'
import { getLatestNewTransmissionEvents } from '../../../../lib/events'
import { makeInput } from './inputs'

// Maps the addresses of on-chain transmitters to display names
const onChainDisplayNames = (
  inputtedTransmitters: string[],
  inputtedDisplayNames: string[],
  onChainTransmitters: PublicKey[],
): string[] => {
  return onChainTransmitters.map((transmitter) => {
    // Find index of on-chain transmitter in inputted transmitters
    const index = inputtedTransmitters.findIndex((inputted) => inputted == transmitter.toString())
    // Use address if display name unknown
    if (index == -1) {
      return transmitter.toString()
    }
    return inputtedDisplayNames[index]
  })
}

export default class OCR2InspectResponses extends SolanaCommand {
  static id = 'ocr2:inspect:responses'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:inspect:responses --network=devnet [AGGREGATOR_ADDRESS]']

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.args[0])
    const onChainState = await program.account.state.fetch(state)

    logger.info(
      `Latest Config: 

    - Latest Transmitter: ${onChainState.config.latestTransmitter}
    - Latest Aggregator Round ID: ${onChainState.config.latestAggregatorRoundId}
    - Latest Config Digest: ${onChainState.config.latestConfigDigest}
    - Latest Config Block Number: ${onChainState.config.latestConfigBlockNumber}
    `,
    )

    const transmitters = onChainState.oracles.xs
      .filter((oracle) => oracle.transmitter._bn != 0)
      .map((oracle) => oracle.transmitter)

    // Get latest transmission events
    const events = await getLatestNewTransmissionEvents(this.provider.connection, state, program)

    // Store observers from each transmission
    const observerRounds: PublicKey[][] = []

    events.forEach((event, i) => {
      // Map observer indices into addresses
      const observers = (event.observers as []).slice(0, event.observerCount).map((observer) => transmitters[observer])
      observerRounds.push(observers as PublicKey[])

      // Log transmission data
      logger.info(
        `Latest Transmission No. ${i + 1}

    - Round Id: ${event.roundId}
    - Config Digest: ${[...event.configDigest]}
    - Answer: ${event.answer}
    - Transmitter: ${observers[event.transmitter]}
    - Observations Timestamp: ${event.observationsTimestamp}
    - Observer Count: ${event.observerCount}
    - Observers: ${observers}
    - Juels Per Lamport: ${event.juelsPerLamport}
    - Reimbursement Gjuels: ${event.reimbursementGjuels}

    ${event.observerCount}/${transmitters.length} oracles are responding
  `,
      )

      // Log oracles that are not responsive
      transmitters.forEach((transmitter) => {
        // If the transmitter is not listed as an observer, log it
        if (!observers.includes(transmitter)) {
          logger.error(
            `Oracle ${transmitter} not responding
      `,
          )
        }
      })
    })

    const inspections: inspection.Inspection[] = []

    // Compare to user/rdd input
    const input = makeInput(this.flags, this.args)
    if (input) {
      logger.info(
        `Comparing On-Chain to Input
      `,
      )
      // Convert on-chain transmitters to display names
      const transmitterDisplayNames = onChainDisplayNames(input.transmitters, input.displayNames, transmitters)
      // Compare transmitters from onchain config to transmitters in RDD
      inspections.push(inspection.makeInspection(transmitterDisplayNames, input.displayNames, 'Transmitters in Config'))
      // For each transmission event, compare observers from transmission to transmitters in RDD
      observerRounds.forEach((observers, i) => {
        // Convert observers to display names
        const observerDisplayNames = onChainDisplayNames(input.transmitters, input.displayNames, observers)
        inspections.push(
          inspection.makeInspection(
            observerDisplayNames,
            input.displayNames,
            `Observers from Latest Transmission No. ${i + 1}`,
          ),
        )
      })
    }

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
