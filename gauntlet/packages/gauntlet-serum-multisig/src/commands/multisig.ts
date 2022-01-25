import { Result } from '@chainlink/gauntlet-core'
import { TransactionResponse, SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair, Transaction } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { ProgramError, parseIdlErrors, Idl, Program } from '@project-serum/anchor'

type ProposalContext = {
  rawTx: RawTransaction
  multisigSigner: PublicKey
  proposalState: any
}

type ProposalAction = (proposal: PublicKey, context: ProposalContext) => Promise<string>

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand
    program: Program<Idl>
    multisigAddress: PublicKey

    static id = `${command.id}`

    constructor(flags, args) {
      super(flags, args)
      logger.info(`Running ${command.id} command using Serum Multisig`)

      this.command = new command(flags, args)
      this.command.invokeMiddlewares(this.command, this.command.middlewares)
      this.require(!!process.env.MULTISIG_ADDRESS, 'Please set MULTISIG_ADDRESS env var')
      this.multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS)
    }

    getRemainingSigners = (proposalState: any, threshold: number): number =>
      Number(threshold) - proposalState.signers.filter(Boolean).length

    isReadyForExecution = (proposalState: any, threshold: number): boolean => {
      return this.getRemainingSigners(proposalState, threshold) <= 0
    }

    execute = async () => {
      const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
      this.program = this.loadProgram(multisig.idl, multisig.programId.toString())
      const [multisigSigner] = await PublicKey.findProgramAddress(
        [this.multisigAddress.toBuffer()],
        this.program.programId,
      )

      const multisigState = await this.program.account.multisig.fetch(process.env.MULTISIG_ADDRESS)
      const threshold = multisigState.threshold
      const owners = multisigState.owners

      logger.info(`Multisig Info:
        - Address: ${this.multisigAddress.toString()}
        - Signer: ${multisigSigner.toString()}
        - Threshold: ${threshold.toString()}
        - Owners: ${owners}`)

      // TODO: Should we support many txs?
      const rawTx = (await this.command.makeRawTransaction(multisigSigner))[0]
      const isCreation = !this.flags.proposal
      if (isCreation) {
        const proposal = await this.createProposalAcount()
        const result = await this.wrapAction(this.createProposal)(proposal, {
          rawTx,
          multisigSigner,
          proposalState: {},
        })
        this.inspectProposalState(proposal, threshold, owners)
        return result
      }

      const proposal = new PublicKey(this.flags.proposal)
      const proposalState = await this.program.account.transaction.fetch(proposal)
      const proposalContext = {
        rawTx,
        multisigSigner,
        proposalState,
      }

      logger.debug(`Proposal state: ${JSON.stringify(proposalState, null, 4)}`)

      const isAlreadyExecuted = proposalState.didExecute
      if (isAlreadyExecuted) {
        logger.info(`Proposal is already executed`)
        return {
          responses: [
            {
              tx: this.wrapResponse('', proposal.toString()),
              contract: proposal.toString(),
            },
          ],
        }
      }

      if (!this.isReadyForExecution(proposalState, threshold)) {
        const result = await this.wrapAction(this.approveProposal)(proposal, proposalContext)
        this.inspectProposalState(proposal, threshold, owners)
        return result
      }
      const result = await this.wrapAction(this.executeProposal)(proposal, proposalContext)
      return result
    }

    wrapAction = (action: ProposalAction) => async (
      proposal: PublicKey,
      context: ProposalContext,
    ): Promise<Result<TransactionResponse>> => {
      const tx = await action(proposal, context)
      return {
        responses: [
          {
            tx: this.wrapResponse(tx, proposal.toString()),
            contract: proposal.toString(),
          },
        ],
      }
    }

    createProposalAcount = async (): Promise<PublicKey> => {
      logger.log('Creating proposal account')
      const proposal = Keypair.generate()
      const txSize = 1300 // Space enough
      const proposalAccount = await this.program.account.transaction.createInstruction(proposal, txSize)
      const accountTx = new Transaction().add(proposalAccount)
      await this.provider.send(accountTx, [proposal, this.wallet.payer])
      logger.success(`Proposal account created at: ${proposal.publicKey.toString()}`)
      return proposal.publicKey
    }

    createProposal: ProposalAction = async (proposal: PublicKey, context): Promise<string> => {
      logger.loading(`Creating proposal`)
      const tx = await this.program.rpc.createTransaction(
        context.rawTx.programId,
        context.rawTx.accounts,
        context.rawTx.data,
        {
          accounts: {
            multisig: this.multisigAddress,
            transaction: proposal,
            proposer: this.wallet.payer.publicKey,
            rent: SYSVAR_RENT_PUBKEY,
          },
          signers: [this.wallet.payer],
        },
      )
      return tx
    }

    approveProposal: ProposalAction = async (proposal: PublicKey): Promise<string> => {
      logger.loading(`Approving proposal`)
      const tx = await this.program.rpc.approve({
        accounts: {
          multisig: this.multisigAddress,
          transaction: proposal,
          owner: this.wallet.publicKey,
        },
      })
      return tx
    }

    executeProposal: ProposalAction = async (proposal: PublicKey, context): Promise<string> => {
      logger.loading(`Executing proposal`)
      // get the command's starting word to map it to the respective IDL(ocr2, store etc)
      const { idl } = getContract(command.id.split(':')[0], '')
      try {
        const tx = await this.program.rpc.executeTransaction({
          accounts: {
            multisig: this.multisigAddress,
            multisigSigner: context.multisigSigner,
            transaction: proposal,
          },
          remainingAccounts: context.proposalState.accounts
            .map((t) => (t.pubkey.equals(context.multisigSigner) ? { ...t, isSigner: false } : t))
            .concat({
              pubkey: context.proposalState.programId,
              isWritable: false,
              isSigner: false,
            }),
        })
        logger.info(`Execution TX hash: ${tx.toString()}`)
        return tx
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

    inspectProposalState = async (proposal, threshold, owners) => {
      const proposalState = await this.program.account.transaction.fetch(proposal)
      logger.debug('Proposal state after action:')
      logger.debug(JSON.stringify(proposalState, null, 4))
      if (proposalState.didExecute == true) {
        logger.info(`Proposal has been executed`)
        return
      }

      if (this.isReadyForExecution(proposalState, threshold)) {
        logger.info(
          `Threshold has been met, an owner needs to run the command once more in order to execute it, with flag --proposal=${proposal}`,
        )
        return
      }
      // inverting the signers boolean array and filtering owners by it
      const remainingEligibleSigners = owners.filter((_, i) => proposalState.signers.map((s) => !s)[i])
      logger.info(
        `${this.getRemainingSigners(
          proposalState,
          threshold,
        )} more owners should sign this proposal, using the same command with flag --proposal=${proposal}`,
      )
      logger.info(`Eligible owners to sign: `)
      logger.info(remainingEligibleSigners.toString())
    }
  }
}
