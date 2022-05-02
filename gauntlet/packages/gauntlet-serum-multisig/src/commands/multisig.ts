import { SolanaCommand, utils, logger } from '@chainlink/gauntlet-solana'
import { BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, Keypair, TransactionInstruction, SystemProgram, AccountMeta } from '@solana/web3.js'
import { Idl, Program } from '@project-serum/anchor'
import { MAX_BUFFER_SIZE } from '../lib/constants'
import { isDeepEqual } from '../lib/utils'
import { CONTRACT_LIST, getContract } from '../lib/contracts'

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

type MultisigState = {
  threshold: number
  owners: PublicKey[]
}

type ProposalState = {
  id: PublicKey
  data: Buffer
  approvers: PublicKey[]
  isExecuted: boolean
  programId: PublicKey
  accounts: AccountMeta[]
}

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

      const multisig = getContract(CONTRACT_LIST.MULTISIG)
      this.program = this.loadProgram(multisig.idl, multisig.programId.toString())

      const signer = this.wallet.publicKey
      const [multisigSigner] = await PublicKey.findProgramAddress(
        [this.multisigAddress.toBuffer()],
        this.program.programId,
      )

      const multisigState = await this.fetchMultisigState(this.multisigAddress)
      this.inspectMsigState(multisigState, multisigSigner)

      const rawTxs = await this.makeRawTransaction(signer)
      // If proposal is not provided, we are at creation time, and a new proposal acc should have been created
      const proposal = new PublicKey(this.flags.proposal || this.flags.multisigProposal || rawTxs[0].keys[1].pubkey)

      if (this.flags.execute) {
        await prompt('Continue?')
        logger.loading(`Executing action...`)
        const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, this.program.idl)(rawTxs)
        logger.success(`TX succeded at ${txhash}`)

        const msigState = await this.fetchMultisigState(this.multisigAddress)
        const proposalState = await this.fetchProposalState(proposal, msigState)
        await this.inspectProposalState(msigState, proposalState)
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
      if (!recentBlock) {
        throw new Error('Block not found. Could not generate message data')
      }
      const tx = utils.makeTx(rawTxs, {
        recentBlockhash: recentBlock.blockhash,
        feePayer: signer,
      })

      const msgData = tx.serializeMessage().toString('base64')
      logger.line()
      logger.success(
        `Message generated with blockhash ID: ${recentBlock.blockhash.toString()} (${new Date(
          recentBlock.blockTime! * 1000,
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

      const multisigState = await this.fetchMultisigState(this.multisigAddress)
      const [multisigSigner] = await PublicKey.findProgramAddress(
        [this.multisigAddress.toBuffer()],
        this.program.programId,
      )

      const instructionIndex = new BN(this.flags.instruction || 0).toNumber()

      // Build the internal command if necessary
      this.command = this.command.buildCommand ? await this.command.buildCommand(this.flags, this.args) : this.command

      // First step should be creating the proposal account. If no proposal flag is provided, proceed to create it
      const proposalFlag = this.flags.proposal || this.flags.multisigProposal
      const proposal = proposalFlag ? new PublicKey(proposalFlag) : await this.createProposalAcount()

      const proposalState = await this.fetchProposalState(proposal, multisigState)
      if (proposalState && proposalState.isExecuted) throw new Error('Multisig Proposal is already executed')
      this.inspectProposalState(multisigState, proposalState)

      const rawTxs = await this.command.makeRawTransaction(multisigSigner)

      logger.loading('Simulating proposal transaction...')
      await this.command.simulateTx(multisigSigner, rawTxs, this.wallet.publicKey)

      if (this.command.beforeExecute) {
        await this.command.beforeExecute(signer)
      }

      await this.showExecutionInstructions(rawTxs, instructionIndex)
      const rawTx = rawTxs[instructionIndex]

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

      this.require(
        await this.isSameProposal(proposalState, rawTx),
        'The transaction generated is different from the Multisig Proposal provided',
      )

      if (!this.isReadyForExecution(proposalState, multisigState.threshold)) {
        return await this.approveProposal(proposal, signer, proposalContext)
      }

      return await this.executeProposal(proposal, signer, proposalContext)
    }

    getRemainingSigners = (proposalState: ProposalState, threshold: number): number =>
      Number(threshold) - proposalState.approvers.filter(Boolean).length

    isReadyForExecution = (proposalState: ProposalState, threshold: number): boolean => {
      return this.getRemainingSigners(proposalState, threshold) <= 0
    }

    fetchMultisigState = async (address: PublicKey): Promise<MultisigState | undefined> => {
      try {
        const state = await this.program.account.multisig.fetch(address)
        return {
          threshold: new BN(state.threshold).toNumber(),
          owners: state.owners.map((owner) => new PublicKey(owner)),
        }
      } catch (e) {
        logger.info('Multisig state not found')
        return
      }
    }

    fetchProposalState = async (
      proposal: PublicKey,
      multisigState: MultisigState,
    ): Promise<ProposalState | undefined> => {
      try {
        const state = await this.program.account.transaction.fetch(proposal)
        return {
          id: proposal,
          data: Buffer.from(state.data),
          approvers: state.signers.reduce((acc, didApprove, i) => {
            if (didApprove) return [...acc, multisigState.owners[i]]
            return acc
          }, []),
          isExecuted: state.didExecute,
          accounts: state.accounts,
          programId: new PublicKey(state.programId),
        }
      } catch (e) {
        logger.info('Multisig Proposal state not found. Should be empty at CREATION time')
        return
      }
    }

    isSameProposal = async (proposalState: ProposalState, rawTx: TransactionInstruction): Promise<boolean> => {
      if (!proposalState) {
        logger.error('Multisig Proposal state does not exist. Considering the proposal as different')
        return false
      }
      const isSameData = Buffer.compare(proposalState.data, rawTx.data) === 0
      const isSameProgramId = new PublicKey(proposalState.programId).toString() === rawTx.programId.toString()
      const isSameAccounts = isDeepEqual(proposalState.accounts, rawTx.keys)
      return isSameData && isSameProgramId && isSameAccounts
    }

    createProposalAcount = async (): Promise<PublicKey> => {
      await prompt('A new Multisig Proposal account will be created. Continue?')
      const proposal = Keypair.generate()
      logger.loading(`Creating Multisig Proposal account at ${proposal.publicKey.toString()}...`)
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

    inspectMsigState = (multisigState: MultisigState, multisigSigner: PublicKey) => {
      logger.info(`Multisig State:
        - Address: ${this.multisigAddress.toString()}
        - Signer: ${multisigSigner.toString()}
        - Threshold: ${multisigState.threshold}
        - Total Owners: ${multisigState.owners.length}
        - Owners List: ${multisigState.owners.map((o) => o.toString())}`)
      logger.line()
    }

    inspectProposalState = (multisigState: MultisigState, proposalState?: ProposalState) => {
      if (!proposalState) return

      logger.info(`Proposal State:
      - Multisig Proposal ID: ${proposalState.id}
      - Total Approvers: ${proposalState.approvers.length}
      - Approvers List: ${proposalState.approvers.map((a) => a.toString())}`)

      if (proposalState.isExecuted) {
        logger.info(`Multisig Proposal has been executed`)
        return
      }

      const proposalId = proposalState.id.toString()
      if (this.isReadyForExecution(proposalState, multisigState.threshold)) {
        logger.info(
          `Threshold has been met, an owner needs to run the command once more in order to execute it, with flag --proposal=${proposalId} or --multisigProposal=${proposalId}`,
        )
        return
      }
      const remainingEligibleSigners = multisigState.owners.filter((owner) => !proposalState.approvers.includes(owner))
      logger.info(
        `${this.getRemainingSigners(
          proposalState,
          multisigState.threshold,
        )} more owners should sign this multisig proposal, using the same command providing ${proposalId} with flag --proposal=${proposalId} or --multisigProposal=${proposalId}`,
      )
      logger.info(`Eligible owners to sign: `)
      logger.info(remainingEligibleSigners.toString())
      logger.line()
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
