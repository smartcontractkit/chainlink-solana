import { Result, WriteCommand } from '@chainlink/gauntlet-core'
import { BpfLoader, BPF_LOADER_PROGRAM_ID, Keypair, LAMPORTS_PER_SOL } from '@solana/web3.js'
import { withProvider, withWallet, withNetwork } from '../middlewares'
import { TransactionResponse } from '../types'
import { Idl, Program, Provider, Wallet } from '@project-serum/anchor'

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
    return {
      hash: '',
      address: programId.publicKey.toString(),
      wait: async (hash) => ({
        success: success,
      }),
    }
  }
}
