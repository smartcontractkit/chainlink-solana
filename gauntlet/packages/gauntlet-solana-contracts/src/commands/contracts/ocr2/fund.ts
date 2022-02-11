import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { utils } from '@project-serum/anchor'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'

export default class Fund extends SolanaCommand {
  static id = 'ocr2:fund'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:fund --network=devnet --state=[ADDRESS] --amount=[AMOUNT]']

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a --state flag with a valid address')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.flags.state)
    const amount = new BN(this.flags.amount)

    const [vaultAuthority, _vaultNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )
    const linkPublicKey = new PublicKey(this.flags.link || process.env.LINK)

    const token = new Token(this.provider.connection, linkPublicKey, TOKEN_PROGRAM_ID, this.wallet.payer)
    const tokenVault = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      linkPublicKey,
      vaultAuthority,
      true,
    )

    const from = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      token.publicKey,
      this.wallet.publicKey,
    )

    logger.loading(`Transferring ${amount} tokens to ${state.toString()} token vault ${tokenVault.toString()}...`)
    const tx = await token.transfer(from, tokenVault, this.wallet.payer, [], amount.toNumber())

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString(), {
            state: state.toString(),
          }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
