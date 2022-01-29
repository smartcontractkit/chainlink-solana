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
  TransactionCtorFields,
} from '@solana/web3.js'
import { withProvider, withWallet, withNetwork } from '../middlewares'
import { RawTransaction, TransactionResponse } from '../types'
import { ProgramError, parseIdlErrors, Idl, Program, Provider } from '@project-serum/anchor'
import { SolanaWallet } from '../wallet'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { makeTx } from '../../lib/utils'

export default abstract class SolanaCommand extends WriteCommand<TransactionResponse> {
  wallet: SolanaWallet
  provider: Provider
  abstract execute: () => Promise<Result<TransactionResponse>>
  makeRawTransaction: (signer: PublicKey) => Promise<RawTransaction[]>

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

  signAndSendRawTx = async (rawTxs: RawTransaction[], extraSigners?: Keypair[]): Promise<TransactionSignature> => {
    const latestSlot = await this.provider.connection.getSlot()
    const recentBlock = await this.provider.connection.getBlock(latestSlot)
    const tx = makeTx(rawTxs, {
      recentBlockhash: recentBlock.blockhash,
      feePayer: this.wallet.publicKey,
    })
    if (extraSigners) {
      tx.sign(...extraSigners)
    }
    const signedTx = await this.wallet.signTransaction(tx)
    logger.loading('Sending tx...')
    return await this.provider.connection.sendRawTransaction(signedTx.serialize())
  }

  withIDL = (action: (...args: any) => Promise<TransactionSignature>, idl: Idl) => async (
    ...args
  ): Promise<TransactionSignature> => {
    try {
      return await action(...args)
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

  sendTx = async (tx: Transaction, signers: Keypair[], idl: Idl): Promise<TransactionSignature> => {
    try {
      return await this.provider.send(tx, signers)
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
