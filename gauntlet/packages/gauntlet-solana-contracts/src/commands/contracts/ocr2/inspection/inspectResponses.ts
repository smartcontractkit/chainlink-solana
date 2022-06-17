import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/solana-gauntlet'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { inspection, logger } from '@chainlink/gauntlet-core/dist/utils'
import { getLatestNewTransmissionEvents } from '../../../../lib/events'
import { toComparablePubKey } from '../../../../lib/inspection'
import RDD from '../../../../lib/rdd'

type Input = {
  description: string
  aggregatorOracles: Oracle[]
}

type Oracle = {
  transmitter: string
  website: string
  name: string
  apis: string[]
}

// Returns a formatted oracle log given a list of addresses and a start tab
// if a list of oracles is provided, add context to logs
const makeOracleLog = (addresses: string[], startTab = 2, oracles?: Oracle[]): string[] => {
  // Returns a default log with only the node address
  const makeDefaultLog = (address: string) => {
    return `
${'  '.repeat(startTab)}- Oracle
${'  '.repeat(startTab + 1)}- Node Address: ${address}`
  }
  // Assemble default logs if no oracles inputted
  if (!oracles) {
    return addresses.map((address) => {
      return makeDefaultLog(address)
    })
  }
  // Assemble logs with context from oracles
  return addresses.map((address) => {
    const oracle = oracles.find((o) => o.transmitter == address)
    // If no oracle found for address, use default log
    if (!oracle) {
      return makeDefaultLog(address)
    }
    return `
${'  '.repeat(startTab)}- ${oracle.name}
${'  '.repeat(startTab + 1)}- Node Address: ${oracle.transmitter}
${'  '.repeat(startTab + 1)}- Website: ${oracle.website}
${'  '.repeat(startTab + 1)}- APIs: ${oracle.apis}`
  })
}

export default class OCR2InspectResponses extends SolanaCommand {
  static id = 'ocr2:inspect:responses'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:inspect:responses --network=devnet --rdd=[PATH_TO_RDD] [AGGREGATOR_ADDRESS]',
    'yarn gauntlet ocr2:inspect:responses [AGGREGATOR_ADDRESS]',
  ]

  makeInput = (userInput): Input | undefined => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''

    // Return empty input if no rdd or user input provided
    if (!rddPath) {
      return undefined
    }

    const rdd = RDD.load(network, rddPath)
    const aggregator = RDD.loadAggregator(this.args[0], network, rddPath)
    const aggregatorOracles: Oracle[] = aggregator.oracles.map((o) => {
      return {
        transmitter: rdd.operators[o.operator].ocrNodeAddress[0],
        website: rdd.operators[o.operator].website,
        name: o.operator,
        apis: o.api,
      }
    })

    return {
      description: aggregator.name,
      aggregatorOracles,
    }
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
      // Assemble logs for observers
      const observerLogs = makeOracleLog(
        observers.map((o) => o.toString()),
        3,
        input?.aggregatorOracles,
      )
      // Log transmission constants
      logger.info(
        `Latest Transmission No. ${i + 1}

    - Round Id: ${event.roundId}
    - Config Digest: ${[...event.configDigest]}
    - Answer: ${event.answer} 
    - Transmitter: ${makeOracleLog([transmitters[event.transmitter].toString()], 3, input?.aggregatorOracles)}
    - Observations Timestamp: ${event.observationsTimestamp}
    - Observer Count: ${event.observerCount}
    - Observers: ${observerLogs}
    - Juels Per Lamport: ${event.juelsPerLamport}
    - Reimbursement Gjuels: ${event.reimbursementGjuels}

    ${event.observerCount}/${transmitters.length} oracles are responding
  `,
      )

      // Log oracles that are not responsive
      var notResponding: number = 0
      transmitters.forEach((transmitter) => {
        // If the transmitter is not listed as an observer, log it
        if (!observers.includes(transmitter)) {
          notResponding++
          logger.error(
            `Not responding ${makeOracleLog([transmitter.toString()], 2, input?.aggregatorOracles)}
            `,
          )
        }
      })
    })

    const inspections: inspection.Inspection[] = []

    if (input) {
      // Compare on-chain transmitters with transmitters from input
      inspections.push(
        inspection.makeInspection(
          transmitters,
          input.aggregatorOracles.map((o) => o.transmitter),
          `On-Chain Transmitters`,
        ),
      )
      // For each transmission event, compare observers from transmission to transmitters from input
      observerRounds.forEach((observers, i) => {
        inspections.push(
          inspection.makeInspection(
            observers,
            input.aggregatorOracles.map((o) => o.transmitter),
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
