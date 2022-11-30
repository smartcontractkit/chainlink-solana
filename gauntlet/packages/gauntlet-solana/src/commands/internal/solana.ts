import { Result, WriteCommand } from '@chainlink/gauntlet-core'
import {
  BpfLoader,
  BPF_LOADER_PROGRAM_ID,
  Keypair,
  LAMPORTS_PER_SOL,
  PublicKey,
  TransactionSignature,
  TransactionInstruction,
  TransactionConfirmationStatus,
  SimulatedTransactionResponse,
  TransactionExpiredBlockheightExceededError,
} from '@solana/web3.js'
import { withProvider, withWallet, withNetwork } from '../middlewares'
import { TransactionResponse } from '../types'
import { ProgramError, parseIdlErrors, Idl, Program, AnchorProvider } from '@project-serum/anchor'
import { SolanaWallet } from '../wallet'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { makeTx } from '../../lib/utils'

/**
 * If transaction is not confirmed by validators in 152 blocks
 * from signing by the wallet
 * it will never reach blockchain and is considered a timeout
 *
 * (e.g. transaction is signed at 121398019 block
 * if its not confirmed by the time blockchain reach 121398171 (121398019 + 152)
 * it will never reach blockchain)
 */
export const MAXIMUM_NUMBER_OF_BLOCKS_FOR_TRANSACTION = 152

async function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export const getUnixTs = () => {
  return new Date().getTime() / 1000
}

const CONFIRM_LEVEL: TransactionConfirmationStatus = 'confirmed'
// const CONFIRM_LEVEL: TransactionConfirmationStatus = 'finalized'

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

  // Based on mango-client-v3
  // - https://github.com/blockworks-foundation/mango-client-v3/blob/d34d248a3c9a97d51d977139f61cd982278b7f01/src/client.ts#L356-L437
  // for more reliable transaction submission.
  signAndSendRawTx = async (
    rawTxs: TransactionInstruction[],
    extraSigners?: Keypair[],
    overrides: {
      units?: number
      price?: number
    } = {},
  ): Promise<TransactionSignature> => {
    if (overrides.units) logger.info(`Sending transaction with custom unit limit: ${overrides.units}`)
    if (overrides.price) logger.info(`Sending transaction with custom unit price: ${overrides.price}`)

    await this.simulateTx(this.wallet.publicKey, rawTxs)

    const currentBlockhash = await this.provider.connection.getLatestBlockhash()

    const tx = makeTx(
      {
        instructions: rawTxs,
        recentBlockhash: currentBlockhash.blockhash,
        payerKey: this.wallet.publicKey,
      },
      overrides,
    )
    if (extraSigners) {
      tx.sign(extraSigners)
    }
    const signedTx = await this.wallet.signVersionedTransaction(tx)
    logger.loading('Sending tx...')

    const rawTransaction = signedTx.serialize()

    const txid = await this.provider.connection.sendRawTransaction(rawTransaction, {
      skipPreflight: true,
    })

    // Send the transaction, periodically retrying for durability
    console.log('Started awaiting confirmation for', txid, 'size:', rawTransaction.length)
    const startTime = getUnixTs()
    let timeout = 60_000
    let done = false
    let retryAttempts = 0
    const retrySleep = 2000
    const maxRetries = 30
    ;(async () => {
      while (!done && getUnixTs() - startTime < timeout / 1000) {
        await sleep(retrySleep)
        // console.log(new Date().toUTCString(), ' sending tx ', txid);
        this.provider.connection.sendRawTransaction(rawTransaction, {
          skipPreflight: true,
        })
        if (retryAttempts <= maxRetries) {
          retryAttempts = retryAttempts++
        } else {
          break
        }
      }
    })()

    try {
      await this.provider.connection.confirmTransaction(
        {
          signature: txid,
          ...currentBlockhash,
        },
        CONFIRM_LEVEL,
      )
    } catch (err: any) {
      if (err instanceof TransactionExpiredBlockheightExceededError) {
        console.log(`Timed out awaiting confirmation. Please confirm in the explorer: `, txid)
      }
      throw err
    } finally {
      done = true
    }
    return txid
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
      const { blockhash } = await this.provider.connection.getLatestBlockhash()
      // TODO: accept a tx pre-made tx without the signatures
      const tx = makeTx({
        instructions: txInstructions,
        recentBlockhash: blockhash,
        payerKey: feePayer || signer,
      })
      // simulating through connection allows to skip signing tx (useful when using Ledger device)
      const { value: simulationResponse } = await this.provider.connection.simulateTransaction(tx, {
        commitment: CONFIRM_LEVEL,
      })
      if (simulationResponse.err) {
        throw new Error(JSON.stringify({ error: simulationResponse.err, logs: simulationResponse.logs }))
      }
      logger.success(`Tx simulation succeeded: ${simulationResponse.unitsConsumed} units consumed.`)
      return simulationResponse.unitsConsumed
    } catch (e) {
      console.log(e.message)
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
}
