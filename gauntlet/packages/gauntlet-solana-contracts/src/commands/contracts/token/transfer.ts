import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST } from '../../../lib/contracts'

export default class TransferToken extends SolanaCommand {
  static id = 'token:transfer'
  static category = CONTRACT_LIST.TOKEN
  static examples = [
    'yarn gauntlet token:transfer --network=devnet --to=AQoKYV7tYpTrFZN6P5oUufbQKAUr9mNYGe1TTJC9wajM --amount=100 7UX2i7SucgLMQcfZ75s3VXmZZY4YRUyJN9X1RgfMoDUi',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('to', 'Provide a token destination address')
    this.requireFlag('amount', 'Provide a token amount')
    this.require(!!args[0], 'Provide a token address')
  }

  execute = async () => {
    const address = this.args[0]
    const token = new Token(this.provider.connection, new PublicKey(address), TOKEN_PROGRAM_ID, this.wallet.payer)

    const from = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      token.publicKey,
      this.wallet.publicKey,
    )

    const destination = (await token.getOrCreateAssociatedAccountInfo(new PublicKey(this.flags.to))).address
    const amount = new BN(this.flags.amount).toNumber()

    logger.loading(`Transferring ${amount} tokens to ${destination}...`)
    const tx = await token.transfer(from, destination, this.wallet.payer, [], amount)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, token.publicKey.toString()),
          contract: token.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
