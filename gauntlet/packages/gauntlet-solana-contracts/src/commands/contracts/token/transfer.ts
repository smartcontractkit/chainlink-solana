import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { createTransferInstruction, getAssociatedTokenAddress } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { TOKEN_DECIMALS } from '../../../lib/constants'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { isValidTokenAccount } from './utils'

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

  makeRawTransaction = async (signer: PublicKey) => {
    const address = this.args[0]

    const token = new PublicKey(address)

    const from = await getAssociatedTokenAddress(token, signer, true)

    const destination = new PublicKey(this.flags.to)
    const amount = BigInt(this.flags.amount) * BigInt(10) ** BigInt(TOKEN_DECIMALS)
    this.require(
      await isValidTokenAccount(this.provider.connection, token, destination),
      `Destination ${destination.toString()} is not a valid token account`,
    )

    logger.info(
      `Preparing instruction to send ${amount.toString()} (${this.flags.amount}) Tokens to ${destination.toString()}`,
    )

    const ix = createTransferInstruction(from, destination, signer, amount)

    return [
      {
        ...ix,
        // createTransferInstruction does not return the PublicKey type
        keys: ix.keys.map((k) => ({ ...k, pubkey: new PublicKey(k.pubkey) })),
      },
    ]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt('Continue sending tokens?')
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Tokens sent on tx hash: ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.args[0]),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
