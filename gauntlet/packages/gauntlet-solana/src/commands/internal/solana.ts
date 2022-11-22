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
  TransactionConfirmationStatus,
  BlockhashWithExpiryBlockHeight,
  RpcResponseAndContext,
  SignatureStatus,
  SimulatedTransactionResponse,
  Connection,
  Commitment,
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

async function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export const getUnixTs = () => {
  return new Date().getTime() / 1000
}

export class TimeoutError extends Error {
  message: string
  txid: string

  constructor({ txid }) {
    super()
    this.message = `Timed out awaiting confirmation. Please confirm in the explorer: `
    this.txid = txid
  }
}

export class SolanaError extends Error {
  message: string
  txid: string

  constructor({ txid, message }) {
    super()
    this.message = message
    this.txid = txid
  }
}

// Workaround vlegacy not supporting commitment on simulateTransaction
export async function simulateTransaction(
  connection: Connection,
  transaction: Transaction,
  commitment: Commitment,
): Promise<RpcResponseAndContext<SimulatedTransactionResponse>> {
  const currentBlockhash = await connection.getLatestBlockhash()
  transaction.recentBlockhash = currentBlockhash.blockhash

  // @ts-ignore
  const wireTransaction = transaction.serialize()
  const encodedTransaction = wireTransaction.toString('base64')
  const config: any = { encoding: 'base64', commitment }
  const args = [encodedTransaction, config]

  // @ts-ignore
  const res = await connection._rpcRequest('simulateTransaction', args)
  if (res.error) {
    throw new Error('failed to simulate transaction: ' + res.error.message)
  }
  return res.result
}

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
  // - https://github.com/blockworks-foundation/mango-client-v3/blob/d34d248a3c9a97d51d977139f61cd982278b7f01/src/client.ts#L546-L668
  // for more reliable transaction submission. Works around https://github.com/solana-labs/solana/issues/25955
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

    // let confirmLevel: TransactionConfirmationStatus = 'confirmed'
    let confirmLevel: TransactionConfirmationStatus = 'finalized'

    this.simulateTx(this.wallet.publicKey, rawTxs)

    const currentBlockhash = await this.provider.connection.getLatestBlockhash()
    const { blockhash, lastValidBlockHeight } = currentBlockhash

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
      tx.sign(...extraSigners) // TODO: partialSign instead?
    }
    const signedTx = await this.wallet.signTransaction(tx)
    logger.loading('Sending tx...')

    const rawTransaction = signedTx.serialize()

    const txid = await this.provider.connection.sendRawTransaction(rawTransaction, {
      skipPreflight: true,
    })

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
      await this.awaitTransactionSignatureConfirmation(txid, timeout, confirmLevel, currentBlockhash)
    } catch (err: any) {
      if (err.timeout) {
        throw new TimeoutError({ txid })
      }
      let simulateResult: SimulatedTransactionResponse | null = null
      try {
        simulateResult = (await this.provider.connection.simulateTransaction(tx)).value
      } catch (e) {
        console.warn('Simulate transaction failed')
      }

      if (simulateResult && simulateResult.err) {
        if (simulateResult.logs) {
          for (let i = simulateResult.logs.length - 1; i >= 0; --i) {
            const line = simulateResult.logs[i]
            if (line.startsWith('Program log: ')) {
              throw new SolanaError({
                message: 'Transaction failed: ' + line.slice('Program log: '.length),
                txid,
              })
            }
          }
        }
        throw new SolanaError({
          message: JSON.stringify(simulateResult.err),
          txid,
        })
      }
      throw new SolanaError({ message: 'Transaction failed', txid })
    } finally {
      done = true
    }
    return txid

    // TODO: skipPreflight: true since we previously simulated it?
    // return await sendAndConfirmRawTransaction(this.provider.connection, rawTransaction) // TODO: { maxRetries: 5 }
  }

  async awaitTransactionSignatureConfirmation(
    txid: TransactionSignature,
    timeout: number,
    confirmLevel: TransactionConfirmationStatus,
    signedAtBlock?: BlockhashWithExpiryBlockHeight,
  ) {
    const timeoutBlockHeight = signedAtBlock
      ? signedAtBlock.lastValidBlockHeight + MAXIMUM_NUMBER_OF_BLOCKS_FOR_TRANSACTION
      : 0
    let startTimeoutCheck = false
    let done = false
    const confirmLevels: (TransactionConfirmationStatus | null | undefined)[] = ['finalized']

    if (confirmLevel === 'confirmed') {
      confirmLevels.push('confirmed')
    } else if (confirmLevel === 'processed') {
      confirmLevels.push('confirmed')
      confirmLevels.push('processed')
    }
    let subscriptionId: number | undefined

    const result = await new Promise((resolve, reject) => {
      ;(async () => {
        setTimeout(() => {
          if (done) {
            return
          }
          if (timeoutBlockHeight !== 0) {
            startTimeoutCheck = true
          } else {
            done = true
            console.log('Timed out for txid: ', txid)
            reject({ timeout: true })
          }
        }, timeout)
        try {
          subscriptionId = this.provider.connection.onSignature(
            txid,
            (result, context) => {
              subscriptionId = undefined
              done = true
              if (result.err) {
                reject(result.err)
              } else {
                // this.lastSlot = context?.slot;
                resolve(result)
              }
            },
            confirmLevel,
          )
        } catch (e) {
          done = true
          console.log('WS error in setup', txid, e)
        }
        let retrySleep = 2000
        while (!done) {
          // eslint-disable-next-line no-loop-func
          await sleep(retrySleep)
          ;(async () => {
            try {
              const promises: [Promise<RpcResponseAndContext<SignatureStatus | null>>, Promise<number>?] = [
                this.provider.connection.getSignatureStatus(txid),
              ]
              //if startTimeoutThreshold passed we start to check if
              //current blocks are did not passed timeoutBlockHeight threshold
              if (startTimeoutCheck) {
                promises.push(this.provider.connection.getBlockHeight(confirmLevel))
              }
              const [signatureStatus, currentBlockHeight] = await Promise.all(promises)
              if (typeof currentBlockHeight !== undefined && timeoutBlockHeight <= currentBlockHeight!) {
                console.log('Timed out for txid: ', txid)
                done = true
                reject({ timeout: true })
              }

              const result = signatureStatus?.value
              if (!done) {
                if (!result) return
                if (result.err) {
                  console.log('REST error for', txid, result)
                  done = true
                  reject(result.err)
                } else if (result.confirmations && confirmLevels.includes(result.confirmationStatus)) {
                  // this.lastSlot = signatureStatuses?.context?.slot;
                  console.log('REST confirmed', txid, result)
                  done = true
                  resolve(result)
                } else {
                  console.log('REST not confirmed', txid, result)
                }
              }
            } catch (e) {
              if (!done) {
                console.log('REST connection error: txid', txid, e)
              }
            }
          })()
        }
      })()
    })

    if (subscriptionId) {
      this.provider.connection.removeSignatureListener(subscriptionId).catch((e) => {
        console.log('WS error in cleanup', e)
      })
    }

    done = true
    return result
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
      // TODO: makeTx is missing extra signatures
      // simulating through connection allows to skip signing tx (useful when using Ledger device)
      // const { value: simulationResponse } = await simulateTransaction(this.provider.connection, tx, 'confirmed')
      const { value: simulationResponse } = await this.provider.connection.simulateTransaction(tx)
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
