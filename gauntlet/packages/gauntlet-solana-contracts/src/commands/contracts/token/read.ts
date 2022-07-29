import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { getMint } from '@solana/spl-token'
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

    const data = await getMint(this.provider.connection, new PublicKey(address))

    console.log(data)

    return {} as Result<TransactionResponse>
  }
}
