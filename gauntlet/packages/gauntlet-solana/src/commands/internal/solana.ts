import { Result, WriteCommand } from '@chainlink/gauntlet-core'
import {
  Transaction,
  BpfLoader,
  BPF_LOADER_PROGRAM_ID,
  Keypair,
  LAMPORTS_PER_SOL,
  PublicKey,
  TransactionSignature,
  TransactionInstruction,
  sendAndConfirmRawTransaction,
  SendTransactionError,
  TransactionExpiredTimeoutError,
} from '@solana/web3.js'

import { withProvider, withWallet, withNetwork } from '../middlewares'
import { TransactionResponse } from '../types'
import { ProgramError, parseIdlErrors, Idl, Program, AnchorProvider } from '@coral-xyz/anchor'
import { SolanaWallet } from '../wallet'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import {
  makeTx,
  percentile,
  validateHistoricalPriorityFeeInput,
  validateRetryPriorityInput,
  Overrides,
} from '../../lib/utils'
import { BlockData, parseBlockFees } from '../../lib/feeUtils'

export default abstract class SolanaCommand extends WriteCommand<TransactionResponse> {
  wallet: SolanaWallet
  provider: AnchorProvider
  program: Program
  abstract execute: () => Promise<Result<TransactionResponse>>
  makeRawTransaction: (signer: PublicKey) => Promise<TransactionInstruction[]>

  buildCommand?: (flags, args) => Promise<SolanaCommand>
  beforeExecute?: (signer: PublicKey) => Promise<void>

  afterExecute = async (response: Result<TransactionResponse>): Promise<void> => {
    logger.success(`Execution finished at transaction: ${response.responses[0].tx.hash}`)
  }

  constructor(flags, args) {
    super(flags, args)
    this.use(withNetwork, withWallet, withProvider)
  }

  static lamportsToSol = (lamports: number) => lamports / LAMPORTS_PER_SOL

  calculatePriceFromHistoricalBlocks = async (numberOfBlocks: number = 1, p: number = 0.5): Promise<number> => {
    const latestSlot = await this.provider.connection.getSlot()
    let prices = []
    let blockData: BlockData
    for (let i = 0; i < numberOfBlocks; i++) {
      const slot = latestSlot - i
      const block = await this.provider.connection.getBlock(slot, {
        maxSupportedTransactionVersion: 0,
      })
      blockData = parseBlockFees(block)
      prices = [...prices, ...blockData.prices]
    }
    // return percentile of priority fess used
    return Math.floor(percentile(prices, p))
  }

  sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

  loadProgram = (idl: Idl, address: string): Program<Idl> => {
    const program = new Program(idl, address, this.provider)
    return program
  }

  wrapResponse = (hash: string, address: string, states?: Record<string, string>): TransactionResponse => ({
    hash: hash,
    address: address,
    states,
    wait: async (hash) => {
      const success = !(await this.provider.connection.confirmTransaction(hash)).value.err
      return { success }
    },
  })

  wrapInspectResponse = (success: boolean, address: string, states?: Record<string, string>): TransactionResponse => ({
    hash: '',
    address,
    states,
    wait: async () => ({ success }),
  })

  deploy = async (bytecode: Buffer | Uint8Array | Array<number>, programId: Keypair): Promise<TransactionResponse> => {
    const success = await BpfLoader.load(
      this.provider.connection,
      this.wallet.payer,
      programId,
      bytecode,
      BPF_LOADER_PROGRAM_ID,
    )
    return {
      hash: '',
      address: programId.publicKey.toString(),
      wait: async (hash) => ({
        success: success,
      }),
    }
  }

  signAndSendRawTx = async (
    rawTxs: TransactionInstruction[],
    extraSigners?: Keypair[],
    overrides: Overrides = {},
    retryCount: number = 0,
  ): Promise<TransactionSignature> => {
    const { blockhash, lastValidBlockHeight } = await this.provider.connection.getLatestBlockhash()
    if (this.flags.priorityFeesConstant && this.flags.priorityFeesHistorical) {
      throw new Error('Cannot have both constant priority fees set and historical priority fees')
    }

    // Default send with priority fees found using block estimator utils if not set
    if (!overrides.price && this.flags.priorityFeesHistorical) {
      validateHistoricalPriorityFeeInput(this.flags.priorityFeesHistorical)
      const [percentile, nBlocks] = this.flags.priorityFeesHistorical.split(',')
      overrides.price = await this.calculatePriceFromHistoricalBlocks(nBlocks, percentile)
      logger.info(
        `Found ${parseFloat(percentile) * 100}th percentile priority fees in past ${nBlocks} blocks as: ${
          overrides.price
        }. Setting as Priority Fee`,
      )
    } else if (!overrides.price && this.flags.priorityFeesConstant) {
      overrides.price = this.flags.priorityFeesConstant
    }
    if (overrides.units) logger.info(`Sending transaction with custom unit limit: ${overrides.units}`)
    if (overrides.price) logger.info(`Sending transaction with custom unit price: ${overrides.price}`)

    const tx = makeTx(
      rawTxs,
      {
        blockhash,
        lastValidBlockHeight,
        feePayer: this.wallet.publicKey,
      },
      overrides,
    )
    if (extraSigners) {
      tx.sign(...extraSigners)
    }
    const signedTx = await this.wallet.signTransaction(tx)
    logger.loading('Sending tx...')
    try {
      return await sendAndConfirmRawTransaction(this.provider.connection, signedTx.serialize())
    } catch (error) {
      // Retry mechanism with greater priority fees
      if (error instanceof SendTransactionError && error.message.includes('congestion')) {
        validateRetryPriorityInput(this.flags.priorityRetry)
        const [percentBump, numberOfRetries] = this.flags.priorityRetry.split(',')
        overrides.price *= 1 + percentBump
        logger.info(
          `Attempt: ${retryCount} - Transaction Failed due to network congestion, increasing by 
          ${parseFloat(1 + percentBump) * 100}% with ${overrides.price} micro Lamports priority fee`,
        )
        if (retryCount < numberOfRetries) {
          return this.signAndSendRawTx(rawTxs, extraSigners, overrides, (retryCount += 1))
        }
        throw error
      } else {
        throw error
      }
    }
  }

  sendTxWithIDL = (sendAction: (...args: any) => Promise<TransactionSignature>, idl: Idl) => async (
    ...args
  ): Promise<TransactionSignature> => {
    try {
      return await sendAction(...args)
    } catch (e) {
      // Translate IDL error
      const idlErrors = parseIdlErrors(idl)
      let translatedErr = ProgramError.parse(e, idlErrors)
      if (translatedErr === null) {
        throw e
      }
      throw translatedErr
    }
  }

  simulateTx = async (signer: PublicKey, txInstructions: TransactionInstruction[], feePayer?: PublicKey) => {
    try {
      const { blockhash, lastValidBlockHeight } = await this.provider.connection.getLatestBlockhash()
      const tx = makeTx(txInstructions, {
        feePayer: feePayer || signer,
        blockhash,
        lastValidBlockHeight,
      })
      // simulating through connection allows to skip signing tx (useful when using Ledger device)
      const { value: simulationResponse } = await this.provider.connection.simulateTransaction(tx)
      if (simulationResponse.err) {
        throw new Error(JSON.stringify({ error: simulationResponse.err, logs: simulationResponse.logs }))
      }
      logger.success(`Tx simulation succeeded: ${simulationResponse.unitsConsumed} units consumed.`)
      return simulationResponse.unitsConsumed
    } catch (e) {
      const parsedError = JSON.parse(e.message)
      const errorCode = parsedError.error.InstructionError ? parsedError.error.InstructionError[1].Custom : -1
      // Insufficient funds error
      if (errorCode == 1 && parsedError.logs.includes('Program log: Error: Insufficient funds')) {
        logger.error('Feed has insufficient funds for transfer')
        // Other errors
      } else {
        logger.error(`Tx simulation failed: ${e.message}`)
      }
      throw e
    }
  }

  sendTx = async (tx: Transaction, signers: Keypair[], idl: Idl): Promise<TransactionSignature> => {
    try {
      return await this.provider.sendAndConfirm(tx, signers)
    } catch (err) {
      // Translate IDL error
      const idlErrors = parseIdlErrors(idl)
      let translatedErr = ProgramError.parse(err, idlErrors)
      if (translatedErr === null) {
        throw err
      }
      throw translatedErr
    }
  }
}
