import { Result } from '@chainlink/gauntlet-core'
import { RawTransaction, SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { AccountMeta, Keypair, PublicKey, SystemProgram, SYSVAR_RENT_PUBKEY } from '@solana/web3.js'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
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

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''
    const aggregator = RDD.loadAggregator(network, rddPath, this.args[0])

    return {
      maxAnswer: aggregator.maxSubmissionValue,
      minAnswer: aggregator.minSubmissionValue,
      transmissions: aggregator.transmissionsAccount,
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  makeRawTransaction = async (signer: PublicKey, state?: PublicKey): Promise<RawTransaction[]> => {
    if (!state) throw new Error('State account is required')

    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    // STATE ACCOUNTS
    const input = this.makeInput(this.flags.input)

    // ARGS
    const [vaultAuthority, vaultNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )

    const linkPublicKey = new PublicKey(this.flags.link || process.env.LINK)
    const requesterAccessController = new PublicKey(this.flags.requesterAccessController || process.env.REQUESTER_ACCESS_CONTROLLER)
    const billingAccessController = new PublicKey(this.flags.billingAccessController || process.env.BILLING_ACCESS_CONTROLLER)

    const minAnswer = new BN(input.minAnswer)
    const maxAnswer = new BN(input.maxAnswer)
    const transmissions = new PublicKey(input.transmissions)

    const tokenVault = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      linkPublicKey,
      vaultAuthority,
      true,
    )

    const data = program.coder.instruction.encode('initialize', {
      nonce: vaultNonce,
      minAnswer,
      maxAnswer,
    })

    const accounts: AccountMeta[] = [
      {
        pubkey: state,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: transmissions,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: signer,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: signer,
        isWritable: false,
        isSigner: true,
      },
      {
        pubkey: linkPublicKey,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: tokenVault,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: vaultAuthority,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: requesterAccessController,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: billingAccessController,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: SYSVAR_RENT_PUBKEY,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: SystemProgram.programId,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: TOKEN_PROGRAM_ID,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: ASSOCIATED_TOKEN_PROGRAM_ID,
        isWritable: false,
        isSigner: false,
      },
    ]

    console.log(`
      STATE ACCOUNTS:
        - State: ${state?.toString()}
        - Transmissions: ${transmissions}
        - Payer: ${this.provider.wallet.publicKey}
        - Owner: ${signer.toString()}
    `)

    const defaultAccountSize = new BN(program.account.state.size)
    const feedCreationInstruction = await SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: state,
      space: defaultAccountSize.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(defaultAccountSize.toNumber()),
      programId: program.programId,
    })

    const rawTxs: RawTransaction[] = [
      {
        data: feedCreationInstruction.data,
        accounts: feedCreationInstruction.keys,
        programId: feedCreationInstruction.programId,
      },
      {
        data,
        accounts,
        programId: program.programId,
      },
    ]

    return rawTxs
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = Keypair.generate()
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey, state.publicKey)
    await prompt(`Start initializing ocr2 feed?`)

    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx, [state])
    logger.success(`Feed initialized on tx ${txhash}`)

    const transmissions = rawTx[1].accounts[1].pubkey

    const [storeAuthority, _storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('store')), state.publicKey.toBuffer()],
      program.programId,
    )

    return {
      data: {
        state: state.publicKey.toString(),
        transmissions: transmissions.toString(),
        storeAuthority: storeAuthority.toString(),
      },
      responses: [
        {
          tx: this.wrapResponse(txhash, address, {
            state: state.publicKey.toString(),
            transmissions: transmissions.toString(),
          }),
          contract: state.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
