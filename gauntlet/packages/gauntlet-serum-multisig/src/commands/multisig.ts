import { Result } from '@chainlink/gauntlet-core'
import { TransactionResponse, SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'

import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { Idl, Program } from '@project-serum/anchor'

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand
    program: Program<Idl>
    proposal: PublicKey
    multisigAddress: PublicKey
    multisigSigner: PublicKey
    owners: [PublicKey]
    threshold: number

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
      const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
      this.program = this.loadProgram(multisig.idl, multisig.programId.toString())
      logger.info(`Multisig Address: ${process.env.MULTISIG_ADDRESS}`)
      const [multisigSigner] = await PublicKey.findProgramAddress(
        [this.multisigAddress.toBuffer()],
        this.program.programId,
      )
      this.multisigSigner = multisigSigner
      logger.info(`Multisig Signer: ${multisigSigner.toString()}`)
      const multisigState = await this.program.account.multisig.fetch(process.env.MULTISIG_ADDRESS)
      logger.debug('Multisig state:')
      logger.debug(JSON.stringify(multisigState, null, 4))
      this.threshold = multisigState.threshold
      this.owners = multisigState.owners

      const isCreation = !this.flags.proposal
      if (isCreation) {
        const rawTxs = await this.command.makeRawTransaction(this.multisigSigner)
        return this.wrapAction(this.createProposal(rawTxs[0]))
      }

      const proposalState = await this.program.account.transaction.fetch(this.flags.proposal)
      logger.debug('Proposal state:')
      logger.debug(JSON.stringify(proposalState, null, 4))

      const isAlreadyExecuted = proposalState.didExecute
      if (isAlreadyExecuted == true) {
        logger.info(`Proposal is already executed`)
        return {} as Result<TransactionResponse>
      }

      const remainingApprovalsNeeded = this.remainingApprovalsNeeded(proposalState, this.threshold)
      const isReadyForExecution = remainingApprovalsNeeded > 0 ? false : true
      if (isReadyForExecution) {
        return this.wrapAction(this.executeProposal(proposalState))
      }

      return this.wrapAction(this.approveProposal())
    }

    wrapAction = async (action) => {
      try {
        const tx = await action
        this.inspectProposalState(this.flags.proposal)
        return {
          responses: [
            {
              tx: this.wrapResponse(tx, this.flags.proposal.toString()),
              contract: this.flags.proposal.toString(),
            },
          ],
        } as Result<TransactionResponse>
      } catch (e) {
        // known errors, defined in multisig contract. see serum_multisig.json
        if (e.code >= 300 && e.code < 400) {
          logger.error(e.msg)
        } else {
          logger.error(e)
        }
      }
    }

    createProposal = async (rawTx: RawTransaction): Promise<string> => {
      logger.loading(`Creating proposal`)
      const txSize = 1000
      const transaction = Keypair.generate()
      this.flags.proposal = transaction.publicKey
      const tx = await this.program.rpc.createTransaction(rawTx.programId, rawTx.accounts, rawTx.data, {
        accounts: {
          multisig: this.multisigAddress,
          transaction: this.flags.proposal,
          proposer: this.wallet.payer.publicKey,
          rent: SYSVAR_RENT_PUBKEY,
        },
        instructions: [await this.program.account.transaction.createInstruction(transaction, txSize)],
        signers: [transaction, this.wallet.payer],
      })
      return tx
    }

    approveProposal = async (): Promise<string> => {
      logger.loading(`Approving proposal`)
      const tx = await this.program.rpc.approve({
        accounts: {
          multisig: this.multisigAddress,
          transaction: this.flags.proposal,
          owner: this.wallet.publicKey,
        },
      })
      return tx
    }

    executeProposal = async (proposalState): Promise<string> => {
      logger.loading(`Executing proposal`)

      const tx = await this.program.rpc.executeTransaction({
        accounts: {
          multisig: this.multisigAddress,
          multisigSigner: this.multisigSigner,
          transaction: this.flags.proposal,
        },
        remainingAccounts: proposalState.accounts
          .map((t) => {
            if (t.pubkey.equals(this.multisigSigner)) {
              return { ...t, isSigner: false }
            }
            return t
          })
          .concat({
            pubkey: proposalState.programId,
            isWritable: false,
            isSigner: false,
          }),
      })
      logger.info(`Execution TX hash: ${tx.toString()}`)
      return tx
    }

    remainingApprovalsNeeded = (proposalState, threshold: number): number => {
      return Number(threshold) - proposalState.signers.filter(Boolean).length
    }

    inspectProposalState = async (proposal) => {
      const proposalState = await this.program.account.transaction.fetch(proposal)
      logger.debug('Proposal state after action:')
      logger.debug(JSON.stringify(proposalState, null, 4))
      if (proposalState.didExecute == true) {
        logger.info(`Proposal has been executed`)
        return
      }

      const remainingApprovalsNeeded = this.remainingApprovalsNeeded(proposalState, this.threshold)

      if (remainingApprovalsNeeded <= 0) {
        logger.info(
          `Threshold has been met, an owner needs to run the command once more in order to execute it, with flag --proposal=${proposal}`,
        )
        return
      }
      // inverting the signers boolean array and filtering owners by it
      const remainingEligibleSigners = this.owners.filter((_, i) => proposalState.signers.map((s) => !s)[i])
      logger.info(
        `${remainingApprovalsNeeded} more owners should sign this proposal, using the same command with flag --proposal=${proposal}`,
      )
      logger.info(`Eligible owners to sign: `)
      logger.info(remainingEligibleSigners.toString())
    }
  }
}
