import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
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
  observation?: number
}

// Returns a formatted oracle log given an address, a command input, a label, and a start tab,
const oracleLog = (address: string, label: string, startTab: number, input?: Input) => {
  const oracle = input?.aggregatorOracles.find((o) => o.transmitter == address)
  if (!oracle) {
    return `
${'  '.repeat(startTab)}- ${label}
${'  '.repeat(startTab + 1)}- Node Address: ${address}`
  }
  return `
${'  '.repeat(startTab)}- ${label}
${'  '.repeat(startTab + 1)}- Node Address: ${address}
${'  '.repeat(startTab + 1)}- Name: ${oracle.name}
${'  '.repeat(startTab + 1)}- Website: ${oracle.website}
${'  '.repeat(startTab + 1)}- APIs: ${oracle.apis}`
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
        payee: rdd.operators[o.operator].adminAddress,
        signer: rdd.operators[o.operator].ocr2OnchainPublicKey[0].substring(14),
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
      const observerLogs = observers.map((observer, j) => oracleLog(observer.toString(), `Observer ${j + 1}`, 3, input))
      // Log transmission constants
      logger.info(
        `Latest Transmission No. ${i + 1}

    - Round Id: ${event.roundId}
    - Config Digest: ${[...event.configDigest]}
    - Answer: ${event.answer} ${oracleLog(transmitters[event.transmitter].toString(), 'Transmitter', 2, input)}
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
            `Not responding ${oracleLog(transmitter, `Oracle ${notResponding}`, 2, input)}
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
