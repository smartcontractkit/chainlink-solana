import { Result } from '@chainlink/gauntlet-core'
import logger from '../logger'
import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { Message, sendAndConfirmRawTransaction, Transaction } from '@solana/web3.js'
import SolanaCommand from './solana'
import { TransactionResponse } from '../types'

export default class SendRawTx extends SolanaCommand {
  static id = 'send_raw_tx'

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('message', 'Include a base64 encoded message')
    this.requireFlag('signature', 'Include a base58 encoded signature')
  }

  execute = async () => {
    const msg = Message.from(Buffer.from(this.flags.message, 'base64'))
    const signature = this.flags.signature

    logger.log('Message', msg)
    logger.log('Signature:', signature)
    const transaction = Transaction.populate(msg, [signature])

    await prompt('Continue sending transaction?')
    const txHash = await sendAndConfirmRawTransaction(this.provider.connection, transaction.serialize())

    logger.success(`Transaction sent with tx hash: ${txHash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txHash, ''),
          contract: '',
        },
      ],
    } as Result<TransactionResponse>
  }
}
