import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey, SystemProgram, SYSVAR_RENT_PUBKEY } from '@solana/web3.js'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { utils } from '@project-serum/anchor'
import BN from 'bn.js'
import { logger } from '@chainlink/gauntlet-core/dist/utils'

export default class Initialize extends SolanaCommand {
  static id = 'ocr2:initialize'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:initialize --network=devnet GMoHmLj8FiBP4XtWBcZVTkVs99DrzQbarLf3bRayfYDg']

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    // STATE ACCOUNTS
    const state = Keypair.generate()
    const transmissions = Keypair.generate()
    const owner = this.wallet.payer

    const linkPublicKey = new PublicKey(this.flags.link)
    const requesterAccessController = new PublicKey(this.flags.requesterAccessController)
    const billingAccessController = new PublicKey(this.flags.billingAccessController)

    // ARGS
    const [vaultAuthority, nonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.publicKey.toBuffer()],
      program.programId,
    )

    const minAnswer = new BN(this.flags.minAnswer)
    const maxAnswer = new BN(this.flags.maxAnswer)
    const decimals = Number(this.flags.decimals || 18)
    const description = this.flags.description || ''

    const tokenVault = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      linkPublicKey,
      vaultAuthority,
      true,
    )

    const accounts = {
      state: state.publicKey,
      transmissions: transmissions.publicKey,
      payer: this.provider.wallet.publicKey,
      owner: owner.publicKey,
      tokenMint: linkPublicKey,
      tokenVault,
      vaultAuthority,
      requesterAccessController,
      billingAccessController,
      rent: SYSVAR_RENT_PUBKEY,
      systemProgram: SystemProgram.programId,
      tokenProgram: TOKEN_PROGRAM_ID,
      associatedTokenProgram: ASSOCIATED_TOKEN_PROGRAM_ID,
    }

    Object.entries(accounts).map(([k, v]) => console.log('KEY:', k, '=', v.toString()))
    console.log(`
      - Min Answer: ${minAnswer.toNumber()}
      - Max Answer: ${maxAnswer.toNumber()}
      - Decimals: ${decimals}
      - Description: ${description}
      - Nonce: ${nonce}
    `)

    logger.loading('Initializing OCR 2 program...')
    const txHash = await program.rpc.initialize(nonce, minAnswer, maxAnswer, decimals, description, {
      accounts,
      signers: [owner, state, transmissions],
      instructions: [
        await program.account.state.createInstruction(state),
        await program.account.transmissions.createInstruction(transmissions),
      ],
    })

    console.log(`
      STATE ACCOUNTS:
        - State: ${state.publicKey}
        - Payer: ${this.provider.wallet.publicKey}
        - Owner: ${owner.publicKey}
    `)

    return {
      responses: [
        {
          tx: this.wrapResponse(txHash, address!, { state: state.publicKey.toString() }),
          contract: state.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
