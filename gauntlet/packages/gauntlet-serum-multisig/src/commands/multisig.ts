import { Result, WriteResponse } from '@chainlink/gauntlet-core'
import { TransactionResponse, SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'

import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { boolean } from '@chainlink/gauntlet-core/dist/lib/args'

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand
    multisigAddress
    multisig
    address
    program
    rawTx
    multisigSigner

    static id = `${command.id}`

    constructor(flags, args) {
      super(flags, args)
      logger.info(`Running ${command.id} command using Serum Multisig`)

      this.command = new command(flags, args)
      this.command.invokeMiddlewares(this.command, this.command.middlewares)
    }

    execute = async () => {
      this.require(process.env.MULTISIG_ADDRESS != null, 'Please set MULTISIG_ADDRESS env var')
      this.multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
      this.multisig = getContract(CONTRACT_LIST.MULTISIG, '')
      this.address = this.multisig.programId.toString()
      this.program = this.loadProgram(this.multisig.idl, this.address)
      logger.info(`Multisig Address: ${process.env.MULTISIG_ADDRESS}`)
      const [multisigSigner] = await PublicKey.findProgramAddress(
        [this.multisigAddress.toBuffer()],
        this.program.programId,
      )
      logger.info(`Multisig Signer: ${multisigSigner.toString()}`)
      this.multisigSigner = multisigSigner
      let multisigState
      let proposalState
      let proposal

      const txResults: string[] = []

      try {
        multisigState = await this.program.account.multisig.fetch(process.env.MULTISIG_ADDRESS)
        logger.debug('Multisig state:')
        logger.debug(JSON.stringify(multisigState, null, 4))
      } catch (e) {
        logger.error(`${process.env.MULTISIG_ADDRESS} is not a valid Multisig.`)
        throw e
      }

      if (this.flags.proposal) {
        try {
          proposalState = await this.program.account.transaction.fetch(this.flags.proposal)
        } catch (e) {
          logger.error(`${this.flags.proposal} is not a valid proposal.`)
          throw e
        }
        proposal = this.flags.proposal
        // Approve a previously created proposal, only if the proposal flag is provided
        const approveTx = await this.approveProposal(proposal)
        txResults.push(approveTx)
      } else {
        // Else create a new proposal TX account. Creator approves it automatically.
        const rawTxs = await this.command.makeRawTransaction(this.multisigSigner)
        const { createTx, proposalTxAccount } = await this.createProposal(rawTxs[0])
        proposal = proposalTxAccount
        txResults.push(createTx)
      }

      // need to re-run again, to get the updated state in case of approval, the call above was for validation, before approval
      proposalState = await this.program.account.transaction.fetch(proposal)
      logger.debug('Proposal state:')
      logger.debug(JSON.stringify(proposalState, null, 4))

      const remainingApprovalsNeeded = Number(multisigState.threshold) - proposalState.signers.filter(Boolean).length

      if (remainingApprovalsNeeded <= 0) {
        try {
          const executeTx = await this.executeProposal(proposal)
          txResults.push(executeTx)
        } catch (e) {
          // known errors, defined in multisig contract. see serum_multisig.json
          if (e.code >= 300 && e.code < 400) {
            logger.error(e.msg)
          } else {
            logger.error(e)
          }
        }
      } else {
        // inverting the signers boolean array and filtering owners by it
        const remainingEligibleSigners = multisigState.owners.filter((_, i) => proposalState.signers.map((s) => !s)[i])
        logger.info(
          `${remainingApprovalsNeeded} more owners should sign this proposal, using the same command with flag --proposal=${proposal}`,
        )
        logger.info(`Eligible owners to sign: `)
        logger.info(remainingEligibleSigners)
      }

      const responses: WriteResponse<TransactionResponse>[] = txResults.map((r) =>
        Object.assign({
          tx: this.wrapResponse(r.toString(), proposal.toString()),
          contract: proposal.toString(),
        }),
      )

      return { responses } as Result<TransactionResponse>
    }

    createProposal = async (rawTx: RawTransaction): Promise<{ createTx: string; proposalTxAccount: PublicKey }> => {
      logger.loading(`Creating proposal`)
      const txSize = 1000
      const transaction = Keypair.generate()
      const proposalTxAccount = transaction.publicKey
      const createTx = await this.program.rpc.createTransaction(rawTx.programId, rawTx.accounts, rawTx.data, {
        accounts: {
          multisig: this.multisigAddress,
          transaction: proposalTxAccount,
          proposer: this.wallet.payer.publicKey,
          rent: SYSVAR_RENT_PUBKEY,
        },
        instructions: [await this.program.account.transaction.createInstruction(transaction, txSize)],
        signers: [transaction, this.wallet.payer],
      })
      return { createTx, proposalTxAccount }
    }

    approveProposal = async (proposal: PublicKey): Promise<string> => {
      logger.loading(`Approving proposal`)
      const approveTx = await this.program.rpc.approve({
        accounts: {
          multisig: this.multisigAddress,
          transaction: proposal,
          owner: this.wallet.publicKey,
        },
      })
      return approveTx
    }

    executeProposal = async (proposal: PublicKey): Promise<string> => {
      logger.loading(`Executing proposal`)
   
      const proposalData = await this.program.account.transaction.fetch(proposal)
      let executeTx
      executeTx = await this.program.rpc.executeTransaction({
        accounts: {
          multisig: this.multisigAddress,
          multisigSigner: this.multisigSigner,
          transaction: proposal,
        },
        remainingAccounts: proposalData.accounts
          .map((t: any) => {
            if (t.pubkey.equals(this.multisigSigner)) {
              return { ...t, isSigner: false }
            }
            return t
          })
          .concat({
            pubkey: proposalData.programId,
            isWritable: false,
            isSigner: false,
          }),
      })
      logger.debug(`Proposal executed. TX hash: ${executeTx.toString()}`)

      return executeTx
    }
  }
}
