import { Result, WriteCommand } from '@chainlink/gauntlet-core'
import {
  BpfLoader,
  BPF_LOADER_PROGRAM_ID,
  Keypair,
  LAMPORTS_PER_SOL,
  PublicKey,
  sendAndConfirmTransaction,
  Transaction,
  TransactionInstruction,
} from '@solana/web3.js'
import { withProvider, withWallet, withNetwork } from '../middlewares'
import { TransactionResponse } from '../types'
import { Coder, Idl, Program, Provider, Wallet } from '@project-serum/anchor'

const BPF_LOADER_PROGRAM_ID_UPGRADEABLE = new PublicKey('BPFLoaderUpgradeab1e11111111111111111111111')
export default abstract class SolanaCommand extends WriteCommand<TransactionResponse> {
  wallet: Wallet
  provider: Provider
  abstract execute: () => Promise<Result<TransactionResponse>>

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

  deploy = async (bytecode: Buffer | Uint8Array | Array<number>, programId: Keypair): Promise<TransactionResponse> => {
    const success = await BpfLoader.load(
      this.provider.connection,
      this.wallet.payer,
      programId,
      bytecode,
      BPF_LOADER_PROGRAM_ID,
    )
    // TODO: How to get deployment hash transaction
    return {
      hash: '',
      address: programId.publicKey.toString(),
      wait: async (hash) => ({
        success: success,
      }),
    }
  }

  // UNUSED
  executeProgram = async (
    contractAddress: string,
    idl: Idl,
    functionName: string,
    params: any,
  ): Promise<TransactionResponse> => {
    const coder = new Coder(idl)
    const encode = (ixName, ix) => coder.instruction.encodeState(ixName, ix)
    /** 
     * An account's public key 
      pubkey: PublicKey;
      /** True if an instruction requires a transaction signature matching `pubkey`
      isSigner: boolean;
      /** True if the `pubkey` can be loaded as a read-write account.
      isWritable: boolean;
      */
    const instruction = new TransactionInstruction({
      keys: [{ pubkey: this.wallet.publicKey, isSigner: false, isWritable: true }],
      programId: new PublicKey(contractAddress),
      data: encode(functionName, params),
    })
    const txHash = await sendAndConfirmTransaction(this.provider.connection, new Transaction().add(instruction), [
      this.wallet.payer,
    ])
    return this.wrapResponse(txHash, contractAddress)
  }
}
