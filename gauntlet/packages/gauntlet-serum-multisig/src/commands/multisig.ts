import { SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair, AccountMeta, Transaction } from '@solana/web3.js'
import { CONTRACT_LIST, getContract, makeTx } from '@chainlink/gauntlet-solana-contracts'
import { Idl, Program } from '@project-serum/anchor'
import { MAX_BUFFER_SIZE } from '../lib/constants'

type ProposalContext = {
  rawTx: RawTransaction
  multisigSigner: PublicKey
  proposalState: any
}

type ProposalAction = (proposal: PublicKey, context: ProposalContext) => Promise<RawTransaction[]>

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
      const rawTxs = await this.makeRawTransaction(multisigSigner)
      const proposal = new PublicKey(this.flags.proposal || rawTxs[0].accounts[1].pubkey)
      const latestSlot = await this.provider.connection.getSlot()
      const recentBlock = await this.provider.connection.getBlock(latestSlot)
      const tx = makeTx(rawTxs, {
        recentBlockhash: recentBlock.blockhash,
        feePayer: new PublicKey(this.flags.feePayer) || this.wallet.payer.publicKey,
      })
      if (this.flags.execute) {
        logger.loading(`Executing action...`)
        const txhash = await this.provider.send(tx, [this.wallet.payer])
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

      logger.log(`Transaction generated:
        ${tx.compileMessage().serialize().toString('base64')}
      `)

      return {
        responses: [
          {
            tx: this.wrapResponse('', this.multisigAddress.toString()),
            contract: this.multisigAddress.toString(),
          },
        ],
      }
    }

    makeRawTransaction = async (signer: PublicKey): Promise<RawTransaction[]> => {
      const multisigState = await this.program.account.multisig.fetch(this.multisigAddress)
      const threshold = multisigState.threshold
      const owners = multisigState.owners

      logger.info(`Multisig Info:
        - Address: ${this.multisigAddress.toString()}
        - Signer: ${signer.toString()}
        - Threshold: ${threshold.toString()}
        - Owners: ${owners}`)

      const instructionIndex = new BN(this.flags.instruction || 0).toNumber()
      const rawTxs = await this.command.makeRawTransaction(signer)
      await this.showExecutionInstructions(rawTxs, instructionIndex)
      const rawTx = rawTxs[instructionIndex]

      // First step should be creating the proposal account. If no proposal flag is provided, proceed to create it
      const proposal = new PublicKey(this.flags.proposal) || (await this.createProposalAcount())

      const proposalState = await this.fetchState(proposal)
      const isCreation = !proposalState
      if (isCreation) {
        return await this.createProposal(proposal, {
          rawTx,
          multisigSigner: signer,
          proposalState: {},
        })
      }
      const proposalContext = {
        rawTx,
        multisigSigner: signer,
        proposalState,
      }

      const isAlreadyExecuted = proposalState.didExecute
      if (isAlreadyExecuted) throw new Error('Proposal is already executed')

      if (!this.isReadyForExecution(proposalState, threshold)) {
        return await this.approveProposal(proposal, proposalContext)
      }
      return await this.executeProposal(proposal, proposalContext)
    }

    fetchState = async (proposal: PublicKey): Promise<any | undefined> => {
      try {
        return await this.program.account.transaction.fetch(proposal)
      } catch (e) {
        logger.info('Proposal state not found')
        return
      }
    }

    createProposalAcount = async (): Promise<PublicKey> => {
      logger.log('Creating proposal account...')
      const proposal = Keypair.generate()
      const txSize = 1300 // Space enough
      const proposalAccount = await this.program.account.transaction.createInstruction(proposal, txSize)
      const accountTx = new Transaction().add(proposalAccount)
      await this.provider.send(accountTx, [proposal, this.wallet.payer])
      logger.success(`Proposal account created at: ${proposal.publicKey.toString()}`)
      return proposal.publicKey
    }

    createProposal: ProposalAction = async (proposal: PublicKey, context): Promise<RawTransaction[]> => {
      logger.loading(`Generating proposal creation data for ${command.id}`)

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
          pubkey: this.wallet.payer.publicKey,
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

    approveProposal: ProposalAction = async (proposal: PublicKey): Promise<RawTransaction[]> => {
      logger.loading(`Generating proposal approval data for ${command.id}`)
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
          pubkey: this.wallet.publicKey,
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

    executeProposal: ProposalAction = async (proposal: PublicKey, context): Promise<RawTransaction[]> => {
      logger.loading(`Generating proposal execution data for ${command.id}`)
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
