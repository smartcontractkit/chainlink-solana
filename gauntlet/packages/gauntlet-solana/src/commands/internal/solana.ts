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
import { makeTx, median, Overrides } from '../../lib/utils'
import { parseBlock } from '../../lib/feeUtils'

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

  calculatePriceFromLatestBlock = async (): Promise<number> => {
    const slot = await this.provider.connection.getSlot()
    const block = await this.provider.connection.getBlock(slot, {
      maxSupportedTransactionVersion: 0,
    })

    const blockData = parseBlock(block)
    // return median of priority fess used
    return median(blockData.prices)
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
    withPriorityFee: Boolean = true,
    overrides: Overrides = {},
  ): Promise<TransactionSignature> => {
    const { blockhash, lastValidBlockHeight } = await this.provider.connection.getLatestBlockhash()

    // Default send with priority fees found using block estimator utils if not set
    if (!overrides.price) {
      overrides.price = await this.calculatePriceFromLatestBlock()
      logger.info(`Found past median priority fees as: ${overrides.price}. Setting as Priority Fee`)
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
      console.log('Error type:', error)
      if (error instanceof SendTransactionError && error.message.includes('congestion') && withPriorityFee) {
        overrides.price += 1000
        logger.info(
          `Transaction Failed due to network congestion, increasing and retrying with ${overrides.price} micro Lamports priority fee`,
        )
        return this.signAndSendRawTx(rawTxs, extraSigners, true, overrides)
      } else if (error instanceof TransactionExpiredTimeoutError) {
        // Sometimes it takes longer to confirm or we need to retry check the transaction
        // Do 3 retries
        const signature = error.signature
        for (let i = 0; i < 3; i++) {
          const status = await this.provider.connection.getSignatureStatus(signature)
          if (status.value && status.value.confirmationStatus == 'confirmed') {
            return signature
          }
          // exponential
          this.sleep(3000 ** i)
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
