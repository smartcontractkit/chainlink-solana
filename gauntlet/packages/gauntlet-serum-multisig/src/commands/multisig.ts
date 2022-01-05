import { Result, WriteResponse } from '@chainlink/gauntlet-core'
import { TransactionResponse, SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'
import { io } from '@chainlink/gauntlet-core/dist/utils'
import { join } from 'path'

import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { AccountMeta, PublicKey, SYSVAR_RENT_PUBKEY, Account, Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../gauntlet-solana-contracts/dist/lib/contracts'
import BN from 'bn.js'

// enum ACTIONS {
//   create = 'create',
//   approve = 'approve',
//   execute = 'execute',
// }

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand
    multisigAddress
    multisig
    address
    program
    rawTx

    static id = `${command.id}`

    constructor(flags, args) {
      super(flags, args)
      logger.info(`Serum multisig wrapping of ${command.id}`)

      this.command = new command(flags, args)
      this.command.invokeMiddlewares(this.command, this.command.middlewares)
  }

    createProposal = async (rawTx: RawTransaction) => {
      logger.loading(`Creating proposal`)
      const txSize = 1000
      const transaction = Keypair.generate()
      const txPublicKey = transaction.publicKey
      const tx = await this.program.rpc.createTransaction(rawTx.programId, rawTx.accounts, rawTx.data, {
        accounts: {
          multisig: this.multisigAddress,
          transaction: txPublicKey,
          proposer: this.wallet.payer.publicKey,
          rent: SYSVAR_RENT_PUBKEY,
        },
        instructions: [await this.program.account.transaction.createInstruction(transaction, txSize)],
        signers: [transaction, this.wallet.payer],
      })
      return {tx, txPublicKey}
    }

    approveProposal = async (txAccount) => {
      logger.loading(`Approving proposal`)
      const tx = await this.program.rpc.approve({
          accounts: {
            multisig: this.multisigAddress,
            transaction: txAccount ,
            owner: this.wallet.publicKey,
          },
        })
      return tx
    }

    executeProposal = async (txAccount) => {
      logger.loading(`Executing proposal`)
      this.require(this.flags.tx != null, "Please set the TX account to execute, using --tx= flag")
      const [multisigSigner] = await PublicKey.findProgramAddress([this.multisigAddress.toBuffer()], this.program.programId)
      const txAccountData = await this.program.account.transaction.fetch(this.flags.tx)
      const tx = await this.program.rpc.executeTransaction({
          accounts: {
            multisig: this.multisigAddress,
            multisigSigner,
            transaction: txAccount,
          },
          remainingAccounts: txAccountData.accounts
            .map((t: any) => {
              if (t.pubkey.equals(multisigSigner)) {
                return { ...t, isSigner: false }
              }
              return t
            })
            .concat({
              pubkey: txAccountData.programId,
              isWritable: false,
              isSigner: false,
            }),
        })
        logger.debug(`TX hash: ${tx.toString()}`)

        return tx
      }

    execute = async () => {
      this.multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
      this.multisig = getContract(CONTRACT_LIST.MULTISIG, '')
      this.address = this.multisig.programId.toString()

      this.program = this.loadProgram(this.multisig.idl, this.address)
      const txResults = []
      if (!this.flags.tx) {
        const rawTxs = await this.command.makeRawTransaction()
        const { tx, txPublicKey } = await this.createProposal(rawTxs[0])
        this.flags.tx = txPublicKey
        logger.info(`TX Account: ${txPublicKey}. This should be used in approve and execute actions by supplying --tx=${txPublicKey} flag`)
        txResults.push(tx)
      }

      if (this.flags.approve) {
        const tx = await this.approveProposal(this.flags.tx)
        txResults.push(tx)
       }

      if (this.flags.execute) {
        const tx = await this.executeProposal(this.flags.tx) 
        txResults.push(tx)
      }

      const responses: WriteResponse<TransactionResponse>[] = txResults.map(r => Object.assign({
        tx: this.wrapResponse(r.toString(), this.flags.tx.toString()),
        contract: this.flags.tx.toString(),
      }))

      return { responses } as Result<TransactionResponse> 

      // const actions = {
      //   [ACTIONS.create]: this.createProposal,
      //   [ACTIONS.approve]: this.approveProposal,
      //   [ACTIONS.execute]: this.executeProposal,
      // }

      // return actions[this.flags.action](this.rawTx)
    }
  }
}

    

