import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Message, Transaction } from '@solana/web3.js'
import { CONTRACT_LIST } from '@chainlink/gauntlet-solana-contracts'

export default class SendRawTx extends SolanaCommand {
  static id = 'send_tx'
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet-serum-multisig send_tx --network=local --data=MULTISIG_ACCOUNT']

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

    logger.log('TRANSACTION:', transaction)

    logger.log('Sending tx...')
    const txHash = await this.provider.connection.sendRawTransaction(transaction.serialize())

    logger.log(txHash)
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
