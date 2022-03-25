import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST } from '../../../lib/contracts'

export default class ReadState extends SolanaCommand {
  static id = 'token:read_state'
  static category = CONTRACT_LIST.TOKEN

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const address = this.args[0]

    const tokenClient = new Token(this.provider.connection, new PublicKey(address), TOKEN_PROGRAM_ID, this.wallet.payer)

    const data = await tokenClient.getMintInfo()

    console.log(data)

    return {
      data: data
    } as Result<TransactionResponse>
  }
}
