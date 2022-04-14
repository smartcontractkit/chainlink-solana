import { Result } from '@chainlink/gauntlet-core'
import { EventParser, BorshCoder, Idl, Event } from '@project-serum/anchor'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { inspection, logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { ORACLES_MAX_LENGTH } from '../../../../lib/constants'

type Input = {}

type NewTransmission = {
  roundId: number
  configDigest: number[]
  answer: BN
  transmitter: PublicKey
  observationsTimestamp: number
  observerCount: number
  observers: PublicKey[]
  juelsPerLamport: BN
  reimbursementGjuels: BN
}

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

  /*
  Searches for latest transmission event from last {numSigs} transactions
  @param programAccount address of ocr2 program account
  @param programId address of ocr2 program
  @param idl interface description for ocr2 contract
  @param transmitters list of oracle addresses from on-chain config
  @param callback function that recieves the NewTransmission
  */
  getLatestTransmissionEvent = async (
    programAccount: PublicKey,
    programId: PublicKey,
    idl: Idl,
    transmitters: PublicKey[],
    callback: (transmission: NewTransmission) => void,
  ) => {
    // Counts number of NewTransmissions found
    var newTransmissionCount: number = 0
    // Tracks the last sig checked
    var lastSigChecked: string | undefined = undefined
    // Number of transactions to check at once
    const batchSize = 100
    // Define coder and event parser
    const coder = new BorshCoder(idl)
    const eventParser = new EventParser(programId, coder)
    // Loop until transmission found
    while (newTransmissionCount == 0) {
      // Get {batchSize} latest sigs
      const sigs = await this.provider.connection.getSignaturesForAddress(programAccount, {
        limit: batchSize,
        before: lastSigChecked,
      })
      // Get the txns associated with the sigs
      const txns = await this.provider.connection.getParsedTransactions(sigs.map((sig) => sig.signature))
      txns.forEach((txn) => {
        // Check txns with logs
        if (txn && txn.meta && txn.meta.logMessages) {
          // Parse logs
          eventParser.parseLogs(txn.meta.logMessages, (event: Event) => {
            // Only check for NewTransmissions when one has not been found
            if (event.name != 'NewTransmission' || newTransmissionCount > 0) {
              return
            }
            // Increment new transmission count
            newTransmissionCount++
            // Return Transmission using callback
            callback({
              roundId: event.data.roundId as number,
              configDigest: event.data.configDigest as number[],
              answer: event.data.answer as BN,
              transmitter: transmitters[event.data.transmitter as number],
              observationsTimestamp: event.data.observationsTimestamp as number,
              observerCount: event.data.observerCount as number,
              observers: (event.data.observers as [])
                .slice(0, event.data.observerCount as number)
                .map((observer) => transmitters[observer]),
              juelsPerLamport: event.data.juelsPerLamport as BN,
              reimbursementGjuels: event.data.reimbursementGjuels as BN,
            })
          })
        }
      })
      // Update last sig checked
      lastSigChecked = sigs[batchSize - 1]
    }
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

    await this.getLatestTransmissionEvent(state, ocr2.programId, ocr2.idl, transmitters, (event: NewTransmission) => {
      // Log transmission data
      logger.info(
        `Latest Transmission
    - Round Id: ${event.roundId}
    - Config Digest: ${event.configDigest}
    - Answer: ${event.answer}
    - Transmitter: ${event.transmitter}
    - Observations Timestamp: ${event.observationsTimestamp}
    - Observer Count: ${event.observerCount}
    - Observers: ${event.observers}
    - Juels Per Lamport: ${event.juelsPerLamport}
    - Reimbursement Gjuels: ${event.reimbursementGjuels}
        `,
      )
      // Log responding oracle count
      logger.info(
        `${event.observerCount}/${transmitters.length} oracles are responding
        `,
      )
      // Log oracles that are not responsive
      transmitters.forEach((transmitter) => {
        // If the transmitter is not listed as an observer, log it
        if (!event.observers.includes(transmitter)) {
          logger.error(
            `Oracle ${transmitter} not responding
            `,
          )
        }
      })
      return
    })

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
