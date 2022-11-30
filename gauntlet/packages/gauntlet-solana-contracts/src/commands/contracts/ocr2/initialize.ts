import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey, TransactionInstruction, SystemProgram } from '@solana/web3.js'
import { getOrCreateAssociatedTokenAccount } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { utils } from '@project-serum/anchor'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import RDD from '../../../lib/rdd'

type Input = {
  minAnswer: number | string
  maxAnswer: number | string
  transmissions: string
}

export default class Initialize extends SolanaCommand {
  static id = 'ocr2:initialize'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:initialize --network=devnet --rdd=[PATH_TO_RDD] [UNDEPLOYED_CONTRACT_ADDRESS]',
    'yarn gauntlet ocr2:initialize [UNDEPLOYED_CONTRACT_ADDRESS]',
  ]

  input: Input

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const aggregator = RDD.loadAggregator(this.args[0], this.flags.network, this.flags.rdd)
    return {
      maxAnswer: aggregator.maxSubmissionValue,
      minAnswer: aggregator.minSubmissionValue,
      transmissions: aggregator.transmissionsAccount,
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)

    return this
  }

  makeRawTransaction = async (signer: PublicKey, state?: PublicKey): Promise<TransactionInstruction[]> => {
    if (!state) throw new Error('State account is required')

    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    // ARGS
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )

    const linkPublicKey = new PublicKey(this.flags.link || process.env.LINK)
    const requesterAccessController = new PublicKey(
      this.flags.requesterAccessController || process.env.REQUESTER_ACCESS_CONTROLLER,
    )
    const billingAccessController = new PublicKey(
      this.flags.billingAccessController || process.env.BILLING_ACCESS_CONTROLLER,
    )

    const minAnswer = new BN(this.input.minAnswer)
    const maxAnswer = new BN(this.input.maxAnswer)
    const transmissions = new PublicKey(this.input.transmissions)

    const tokenVault = (
      await getOrCreateAssociatedTokenAccount(
        this.provider.connection,
        this.wallet.payer,
        linkPublicKey,
        vaultAuthority,
        true,
      )
    ).address

    const tx = await program.methods
      .initialize(minAnswer, maxAnswer)
      .accounts({
        state,
        feed: transmissions,
        owner: signer,
        tokenMint: linkPublicKey,
        tokenVault,
        vaultAuthority,
        requesterAccessController,
        billingAccessController,
      })
      .instruction()

    console.log(`
      STATE ACCOUNTS:
        - State: ${state?.toString()}
        - Transmissions: ${transmissions}
        - Payer: ${this.wallet.publicKey}
        - Owner: ${signer.toString()}
    `)

    const defaultAccountSize = new BN(program.account.state.size)
    const feedCreationInstruction = SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: state,
      space: defaultAccountSize.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(defaultAccountSize.toNumber()),
      programId: program.programId,
    })

    return [feedCreationInstruction, tx]
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)

    const state = Keypair.generate()
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey, state.publicKey)
    await prompt(`Start initializing ocr2 feed?`)

    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, this.program.idl)(rawTx, [state])
    logger.success(`Feed initialized on tx ${txhash}`)

    const [storeAuthority, _storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('store')), state.publicKey.toBuffer()],
      this.program.programId,
    )

    return {
      data: {
        state: state.publicKey.toString(),
        transmissions: this.input.transmissions.toString(),
        storeAuthority: storeAuthority.toString(),
      },
      responses: [
        {
          tx: this.wrapResponse(txhash, state.publicKey.toString(), {
            state: state.publicKey.toString(),
            transmissions: this.input.transmissions.toString(),
          }),
          contract: state.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
