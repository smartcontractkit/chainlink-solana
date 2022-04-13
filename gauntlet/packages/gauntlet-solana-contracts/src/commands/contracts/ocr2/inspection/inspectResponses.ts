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
  Gets latest NewTransmission events from latest config block number
  @param blockNumber block to query
  @param programId address of ocr2 program
  @param programAccount address of ocr2 program account
  @param idl interface description for ocr2 contract
  @param transmitters list of oracle addresses from on-chain config
  */
  getLatestTransmissionEvent = async (
    blockNumber: number,
    programId: PublicKey,
    programAccount: PublicKey,
    idl: Idl,
    transmitters: PublicKey[],
  ) => {
    // Event parser callback (used by parseLogs)
    const logTransmissionEvent = (event: Event) => {
      // Assemble NewTransmission struct
      if (event.name == 'NewTransmission') {
        const transmission: NewTransmission = {
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
        }
        // Log transmission data
        logger.info(`Latest Transmission
  - Round Id: ${transmission.roundId}
  - Config Digest: ${transmission.configDigest}
  - Answer: ${transmission.answer}
  - Transmitter: ${transmission.transmitter}
  - Observations Timestamp: ${transmission.observationsTimestamp}
  - Observer Count: ${transmission.observerCount}
  - Observers: ${transmission.observers}
  - Juels Per Lamport: ${transmission.juelsPerLamport}
  - Reimbursement Gjuels: ${transmission.reimbursementGjuels}
  `)
        // Log responding oracle count
        logger.info(`${transmission.observerCount}/${transmitters.length} oracles are responding
        `)
        // Log oracles that are not responsive
        transmitters.forEach((transmitter) => {
          // If the transmitter is not listed as an observer, log it
          if (!transmission.observers.includes(transmitter)) {
            logger.error(`Oracle ${transmitter} not responding
            `)
          }
        })
      }
    }

    // Define coder and event parser
    const coder = new BorshCoder(idl)
    const eventParser = new EventParser(programId, coder)
    // Get block
    let block = await this.provider.connection.getBlock(blockNumber)
    if (!block) {
      throw new Error('Block not found. Could not find latest block number in config')
    }
    // Iterate over all transactions in block
    block.transactions.forEach((transaction) => {
      // Get list of accounts keys associated with txn
      let accountKeys = transaction.transaction.message.accountKeys.map((key) => `${key}`)
      // Get list of instructions associated with txn
      let instructions = transaction.transaction.message.instructions
      // Check each instruction for program id and account
      instructions.forEach((instruction) => {
        // Instruction's program id == inputted program id
        let hasProgramId = accountKeys[instruction.programIdIndex] == programId.toString()
        // Transaction's account keys contains inputted account key
        let hasAccount = accountKeys.includes(programAccount.toString())
        // Parse event logs if above conditions are met
        if (hasProgramId && hasAccount && transaction.meta && transaction.meta.logMessages) {
          eventParser.parseLogs(transaction.meta.logMessages, logTransmissionEvent)
        }
      })
    })
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

    this.getLatestTransmissionEvent(
      onChainState.config.latestConfigBlockNumber.toNumber(),
      ocr2.programId,
      state,
      ocr2.idl,
      onChainState.oracles.xs
        .map((oracle) => oracle.transmitter)
        .filter((transmitter) => {
          return transmitter._bn != 0
        }),
    )

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
