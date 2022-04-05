import { SolanaCommand } from '@chainlink/gauntlet-solana'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, Keypair, TransactionInstruction, SystemProgram } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../lib/contracts'
import { makeTx } from '../lib/utils'
import { Idl, Program } from '@project-serum/anchor'
import { MAX_BUFFER_SIZE } from '../lib/constants'
import { isDeepEqual } from '../lib/utils'

type ProposalContext = {
  rawTx: TransactionInstruction
  multisigSigner: PublicKey
  proposalState: any
}

type ProposalAction = (
  proposal: PublicKey,
  signer: PublicKey,
  context: ProposalContext,
) => Promise<TransactionInstruction[]>

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand
    program: Program<Idl>
    multisigAddress: PublicKey

    static id = `${command.id}:multisig`
    static category = command.category

    constructor(flags, args) {
      super(flags, args)
      logger.info(`Running ${command.id} command using Serum Multisig`)

      this.command = new command({ ...flags, bufferSize: MAX_BUFFER_SIZE }, args)
      this.require(!!process.env.MULTISIG_ADDRESS, 'Please set MULTISIG_ADDRESS env var')
      this.multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS!)
    }

    execute = async () => {
      // TODO: Command underneath will try to load its own provider and wallet if invoke middlewares, but we should be able to specify which ones to use, in an obvious better way
      this.command.provider = this.provider
      this.command.wallet = this.wallet

      const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
      this.program = this.loadProgram(multisig.idl, multisig.programId.toString())

      const signer = this.wallet.publicKey
      const rawTxs = await this.makeRawTransaction(signer)
      // If proposal is not provided, we are at creation time, and a new proposal acc should have been created
      const proposal = new PublicKey(this.flags.proposal || this.flags.multisigProposal || rawTxs[0].keys[1].pubkey)

      if (this.flags.execute) {
        await prompt('Continue?')
        logger.loading(`Executing action...`)
        const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, this.program.idl)(rawTxs)
        logger.success(`TX succeded at ${txhash}`)
        await this.inspectProposalState(proposal)
        return {
          responses: [
            {
              tx: this.wrapResponse(txhash, this.multisigAddress.toString()),
              contract: this.multisigAddress.toString(),
            },
          ],
        }
      }

      const latestSlot = await this.provider.connection.getSlot()
      const recentBlock = await this.provider.connection.getBlock(latestSlot)
      const tx = makeTx(rawTxs, {
        recentBlockhash: recentBlock!.blockhash,
        feePayer: signer,
      })

      const msgData = tx.serializeMessage().toString('base64')
      logger.line()
      logger.success(
        `Message generated with blockhash ID: ${recentBlock!.blockhash.toString()} (${new Date(
          recentBlock!.blockTime! * 1000,
        ).toLocaleString()}). MESSAGE DATA:`,
      )
      logger.log()
      logger.log(msgData)
      logger.log()
      logger.line()

      return {
        responses: [
          {
            tx: this.wrapResponse('', this.multisigAddress.toString()),
            contract: this.multisigAddress.toString(),
            data: {
              transactionData: msgData,
            },
          },
        ],
      }
    }

    makeRawTransaction = async (signer: PublicKey): Promise<TransactionInstruction[]> => {
      logger.info(`Generating transaction data using ${signer.toString()} account as signer`)

      const multisigState = await this.program.account.multisig.fetch(this.multisigAddress)
      const [multisigSigner] = await PublicKey.findProgramAddress(
        [this.multisigAddress.toBuffer()],
        this.program.programId,
      )
      const threshold = multisigState.threshold
      const owners = multisigState.owners

      logger.info(`Multisig Info:
        - Address: ${this.multisigAddress.toString()}
        - Signer: ${multisigSigner.toString()}
        - Threshold: ${threshold.toString()}
        - Owners: ${owners}`)

      const instructionIndex = new BN(this.flags.instruction || 0).toNumber()

      // Build the internal command if necessary
      this.command = this.command.buildCommand ? await this.command.buildCommand(this.flags, this.args) : this.command
      const rawTxs = await this.command.makeRawTransaction(multisigSigner)
      await this.command.simulateTx(multisigSigner, rawTxs, this.wallet.publicKey)
      await this.showExecutionInstructions(rawTxs, instructionIndex)
      const rawTx = rawTxs[instructionIndex]

      // First step should be creating the proposal account. If no proposal flag is provided, proceed to create it
      const proposalFlag = this.flags.proposal || this.flags.multisigProposal
      const proposal = proposalFlag ? new PublicKey(proposalFlag) : await this.createProposalAcount()

      if (this.command.beforeExecute) {
        await this.command.beforeExecute(signer)
      }

      const proposalState = await this.fetchState(proposal)
      const isCreation = !proposalState
      if (isCreation) {
        return await this.createProposal(proposal, signer, {
          rawTx,
          multisigSigner,
          proposalState: {},
        })
      }
      const proposalContext = {
        rawTx,
        multisigSigner,
        proposalState,
      }

      const isAlreadyExecuted = proposalState.didExecute
      if (isAlreadyExecuted) throw new Error('Multisig Proposal is already executed')

      this.require(
        await this.isSameProposal(proposal, rawTx),
        'The transaction generated is different from the Multisig Proposal provided',
      )

      if (!this.isReadyForExecution(proposalState, threshold)) {
        return await this.approveProposal(proposal, signer, proposalContext)
      }

      return await this.executeProposal(proposal, signer, proposalContext)
    }

    getRemainingSigners = (proposalState: any, threshold: number): number =>
      Number(threshold) - proposalState.signers.filter(Boolean).length

    isReadyForExecution = (proposalState: any, threshold: number): boolean => {
      return this.getRemainingSigners(proposalState, threshold) <= 0
    }

    fetchState = async (proposal: PublicKey): Promise<any | undefined> => {
      try {
        return await this.program.account.transaction.fetch(proposal)
      } catch (e) {
        logger.info('Multisig Proposal state not found. Should be empty at CREATION time')
        return
      }
    }

    isSameProposal = async (proposal: PublicKey, rawTx: TransactionInstruction): Promise<boolean> => {
      const state = await this.fetchState(proposal)
      if (!state) {
        logger.error('Multisig Proposal state does not exist. Considering the proposal as different')
        return false
      }
      const isSameData = Buffer.compare(state.data, rawTx.data) === 0
      const isSameProgramId = new PublicKey(state.programId).toString() === rawTx.programId.toString()
      const isSameAccounts = isDeepEqual(state.accounts, rawTx.keys)
      return isSameData && isSameProgramId && isSameAccounts
    }

    createProposalAcount = async (): Promise<PublicKey> => {
      await prompt('A new Multisig Proposal account will be created. Continue?')
      logger.log('Creating Multisig Proposal account...')
      const proposal = Keypair.generate()
      const txSize = 1300 // Space enough
      const proposalInstruction = await SystemProgram.createAccount({
        fromPubkey: this.wallet.publicKey,
        newAccountPubkey: proposal.publicKey,
        space: txSize,
        lamports: await this.provider.connection.getMinimumBalanceForRentExemption(txSize),
        programId: this.program.programId,
      })
      await this.signAndSendRawTx([proposalInstruction], [proposal])
      logger.success(`Multisig Proposal account created at: ${proposal.publicKey.toString()}`)
      return proposal.publicKey
    }

    createProposal: ProposalAction = async (
      proposal: PublicKey,
      signer,
      context,
    ): Promise<TransactionInstruction[]> => {
      logger.loading(`Generating Multisig Proposal CREATION data for ${command.id}`)

      const tx = this.program.instruction.createTransaction(
        context.rawTx.programId,
        context.rawTx.keys,
        context.rawTx.data,
        {
          accounts: {
            multisig: this.multisigAddress,
            transaction: proposal,
            proposer: signer,
          },
        },
      )
      return [tx]
    }

    approveProposal: ProposalAction = async (proposal: PublicKey, signer): Promise<TransactionInstruction[]> => {
      logger.loading(`Generating Multisig Proposal APPROVAL data for ${command.id}`)

      const tx = this.program.instruction.approve({
        accounts: {
          multisig: this.multisigAddress,
          transaction: proposal,
          owner: signer,
        },
      })
      return [tx]
    }

    executeProposal: ProposalAction = async (
      proposal: PublicKey,
      signer,
      context,
    ): Promise<TransactionInstruction[]> => {
      logger.loading(`Generating Multisig Proposal EXECUTION data for ${command.id}`)

      const remainingAccounts = context.proposalState.accounts
        .map((t) => (t.pubkey.equals(context.multisigSigner) ? { ...t, isSigner: false } : t))
        .concat({
          pubkey: context.proposalState.programId,
          isWritable: false,
          isSigner: false,
        })

      const tx = this.program.instruction.executeTransaction({
        accounts: {
          multisig: this.multisigAddress,
          transaction: proposal,
          multisigSigner: context.multisigSigner,
        },
        remainingAccounts,
      })
      return [tx]
    }

    inspectProposalState = async (proposal) => {
      const proposalState = await this.program.account.transaction.fetch(proposal)
      const multisigState = await this.program.account.multisig.fetch(this.multisigAddress)
      const threshold = multisigState.threshold
      const owners = multisigState.owners

      logger.debug('Multisig Proposal state after action:')
      logger.debug(JSON.stringify(proposalState, null, 4))
      if (proposalState.didExecute == true) {
        logger.info(`Multisig Proposal has been executed`)
        return
      }

      if (this.isReadyForExecution(proposalState, threshold)) {
        logger.info(
          `Threshold has been met, an owner needs to run the command once more in order to execute it, with flag --proposal=${proposal} or --multisigProposal=${proposal}`,
        )
        return
      }
      // inverting the signers boolean array and filtering owners by it
      const remainingEligibleSigners = owners.filter((_, i) => proposalState.signers.map((s) => !s)[i])
      logger.info(
        `${this.getRemainingSigners(
          proposalState,
          threshold,
        )} more owners should sign this multisig proposal, using the same command providing ${proposal} with flag --proposal=${proposal} or --multisigProposal=${proposal}`,
      )
      logger.info(`Eligible owners to sign: `)
      logger.info(remainingEligibleSigners.toString())
    }

    showExecutionInstructions = async (rawTxs: TransactionInstruction[], instructionIndex: number) => {
      logger.info(`Execution Information:
        The command ${command.id} with multisig takes up to ${rawTxs.length} (${rawTxs.map(
        (_, i) => i,
      )}) zero-indexed transactions.
        Currently running ${instructionIndex + 1} of ${rawTxs.length}.
      `)
    }
  }
}
