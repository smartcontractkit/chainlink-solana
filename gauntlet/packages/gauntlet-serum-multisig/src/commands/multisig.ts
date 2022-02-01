import { SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair, AccountMeta, SystemProgram } from '@solana/web3.js'
import { CONTRACT_LIST, getContract, makeTx } from '@chainlink/gauntlet-solana-contracts'
import { Idl, Program } from '@project-serum/anchor'
import { MAX_BUFFER_SIZE } from '../lib/constants'

type ProposalContext = {
  rawTx: RawTransaction
  multisigSigner: PublicKey
  proposalState: any
}

type ProposalAction = (proposal: PublicKey, signer: PublicKey, context: ProposalContext) => Promise<RawTransaction[]>

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand
    program: Program<Idl>
    multisigAddress: PublicKey

    static id = `${command.id}`

    constructor(flags, args) {
      super(flags, args)
      logger.info(`Running ${command.id} command using Serum Multisig`)

      this.command = new command({ ...flags, bufferSize: MAX_BUFFER_SIZE }, args)
      this.require(!!process.env.MULTISIG_ADDRESS, 'Please set MULTISIG_ADDRESS env var')
      this.multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS)
    }

    execute = async () => {
      // TODO: Command underneath will try to load its own provider and wallet if invoke middlewares, but we should be able to specify which ones to use, in an obvious better way
      this.command.provider = this.provider
      this.command.wallet = this.wallet

      const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
      this.program = this.loadProgram(multisig.idl, multisig.programId.toString())

      // If execute flag is provided, a signer cannot, as the Keypair is required
      const isInvalidSigner = !!this.flags.execute && !!this.flags.signer
      this.require(!isInvalidSigner, 'To execute the transaction the signer must be loaded from a wallet')
      const signer = new PublicKey(this.flags.signer || this.wallet.publicKey)
      const rawTxs = await this.makeRawTransaction(signer)
      // If proposal is not provided, we are at creation time, and a new proposal acc should have been created
      const proposal = new PublicKey(this.flags.proposal || rawTxs[0].accounts[1].pubkey)

      if (this.flags.execute) {
        await prompt('CREATION,APPROVAL or EXECUTION TX will be executed. Continue?')
        logger.loading(`Executing action...`)
        const txhash = await this.withIDL(this.signAndSendRawTx, this.program.idl)(rawTxs)
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
        recentBlockhash: recentBlock.blockhash,
        feePayer: signer,
      })

      const msgData = tx.serializeMessage().toString('base64')
      logger.line()
      logger.success(
        `Message generated with blockhash ID: ${recentBlock.blockhash.toString()} (${new Date(
          recentBlock.blockTime * 1000,
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

    makeRawTransaction = async (signer: PublicKey): Promise<RawTransaction[]> => {
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
      const rawTxs = await this.command.makeRawTransaction(multisigSigner)
      await this.showExecutionInstructions(rawTxs, instructionIndex)
      const rawTx = rawTxs[instructionIndex]

      // First step should be creating the proposal account. If no proposal flag is provided, proceed to create it
      const proposal = this.flags.proposal ? new PublicKey(this.flags.proposal) : await this.createProposalAcount()

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
      if (isAlreadyExecuted) throw new Error('Proposal is already executed')

      this.require(
        await this.isSameProposal(proposal, rawTx),
        'The transaction generated is different from the proposal provided',
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
        logger.info('Proposal state not found. Should be empty at CREATION time')
        return
      }
    }

    isSameProposal = async (proposal: PublicKey, rawTx: RawTransaction): Promise<boolean> => {
      const state = await this.fetchState(proposal)
      if (!state) {
        logger.error('Proposal state does not exist. Considering the proposal as different')
        return false
      }
      const isSameData = Buffer.compare(state.data, rawTx.data) === 0
      const isSameProgramId = new PublicKey(state.programId).toString() === rawTx.programId.toString()
      const isSameAccounts = JSON.stringify(state.accounts) === JSON.stringify(rawTx.accounts)
      return isSameData && isSameProgramId && isSameAccounts
    }

    createProposalAcount = async (): Promise<PublicKey> => {
      await prompt('A new proposal account will be created. Continue?')
      logger.log('Creating proposal account...')
      const proposal = Keypair.generate()
      const txSize = 1300 // Space enough
      const proposalInstruction = await SystemProgram.createAccount({
        fromPubkey: this.wallet.publicKey,
        newAccountPubkey: proposal.publicKey,
        space: txSize,
        lamports: await this.provider.connection.getMinimumBalanceForRentExemption(txSize),
        programId: this.program.programId,
      })
      const rawTx: RawTransaction = {
        data: proposalInstruction.data,
        accounts: proposalInstruction.keys,
        programId: proposalInstruction.programId,
      }
      await this.signAndSendRawTx([rawTx], [proposal])
      logger.success(`Proposal account created at: ${proposal.publicKey.toString()}`)
      return proposal.publicKey
    }

    createProposal: ProposalAction = async (proposal: PublicKey, signer, context): Promise<RawTransaction[]> => {
      logger.loading(`Generating proposal CREATION data for ${command.id}`)

      const data = this.program.coder.instruction.encode('createTransaction', {
        pid: context.rawTx.programId,
        accs: context.rawTx.accounts,
        data: context.rawTx.data,
      })
      const accounts: AccountMeta[] = [
        {
          pubkey: this.multisigAddress,
          isWritable: false,
          isSigner: false,
        },
        {
          pubkey: proposal,
          isWritable: true,
          isSigner: false,
        },
        {
          pubkey: signer,
          isWritable: false,
          isSigner: true,
        },
        {
          pubkey: SYSVAR_RENT_PUBKEY,
          isWritable: false,
          isSigner: false,
        },
      ]
      const rawTx: RawTransaction = {
        data,
        accounts,
        programId: this.program.programId,
      }
      return [rawTx]
    }

    approveProposal: ProposalAction = async (proposal: PublicKey, signer): Promise<RawTransaction[]> => {
      logger.loading(`Generating proposal APPROVAL data for ${command.id}`)

      const data = this.program.coder.instruction.encode('approve', {})
      const accounts: AccountMeta[] = [
        {
          pubkey: this.multisigAddress,
          isWritable: false,
          isSigner: false,
        },
        {
          pubkey: proposal,
          isWritable: true,
          isSigner: false,
        },
        {
          pubkey: signer,
          isWritable: false,
          isSigner: true,
        },
      ]
      const rawTx: RawTransaction = {
        data,
        accounts,
        programId: this.program.programId,
      }
      return [rawTx]
    }

    executeProposal: ProposalAction = async (proposal: PublicKey, signer, context): Promise<RawTransaction[]> => {
      logger.loading(`Generating proposal EXECUTION data for ${command.id}`)

      const data = this.program.coder.instruction.encode('executeTransaction', {})
      const remainingAccounts = context.proposalState.accounts
        .map((t) => (t.pubkey.equals(context.multisigSigner) ? { ...t, isSigner: false } : t))
        .concat({
          pubkey: context.proposalState.programId,
          isWritable: false,
          isSigner: false,
        })
      const accounts: AccountMeta[] = [
        {
          pubkey: this.multisigAddress,
          isWritable: false,
          isSigner: false,
        },
        {
          pubkey: context.multisigSigner,
          isWritable: false,
          isSigner: false,
        },
        {
          pubkey: proposal,
          isWritable: true,
          isSigner: false,
        },
        ...remainingAccounts,
      ]
      const rawTx: RawTransaction = {
        data,
        accounts,
        programId: this.program.programId,
      }
      return [rawTx]
    }

    inspectProposalState = async (proposal) => {
      const proposalState = await this.program.account.transaction.fetch(proposal)
      const multisigState = await this.program.account.multisig.fetch(this.multisigAddress)
      const threshold = multisigState.threshold
      const owners = multisigState.owners

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

    showExecutionInstructions = async (rawTxs: RawTransaction[], instructionIndex: number) => {
      logger.info(`Execution Information:
        The command ${command.id} with multisig takes up to ${rawTxs.length} (${rawTxs.map(
        (_, i) => i,
      )}) zero-indexed transactions.
        Currently running ${instructionIndex + 1} of ${rawTxs.length}.
      `)
    }
  }
}
