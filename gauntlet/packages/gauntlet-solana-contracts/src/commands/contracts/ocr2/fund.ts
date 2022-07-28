import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'

export default class Fund extends SolanaCommand {
  static id = 'ocr2:fund'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:fund --network=devnet --amount=[AMOUNT] [AGGREGATOR_ADDRESS]']

  constructor(flags, args) {
    super(flags, args)

    this.requireArgs('Please provide an aggregator address')
    this.requireFlag('amount', 'Provide an --amount flag')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.args[0])
    const amount = new BN(this.flags.amount)

    const linkPublicKey = new PublicKey(this.flags.link || process.env.LINK)

    // Resolve the tokenVault from the aggregator state account
    const stateAccount = await program.account.state.fetch(state) as any
    const tokenVault = stateAccount.config.tokenVault
    const tokenMint = stateAccount.config.tokenMint
    this.require(tokenMint.equals(linkPublicKey), 'LINK does not match aggregator.config.tokenMint')

    const token = new Token(this.provider.connection, linkPublicKey, TOKEN_PROGRAM_ID, this.wallet.payer)

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
