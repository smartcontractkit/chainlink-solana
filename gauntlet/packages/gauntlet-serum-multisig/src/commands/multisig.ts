import { Result } from '@chainlink/gauntlet-core'
import { TransactionResponse, SolanaCommand } from '@chainlink/gauntlet-solana'

/*
  1. Create Tx
  2. Approve Tx
  3. Execute Tx

*/

class Multisig extends SolanaCommand {
  command: SolanaCommand

  constructor(flags, args, command) {
    super(flags, args)

    const instance = new command(flags, args)
    this.command = instance.invokeMiddlewares(this.command, this.command.middlewares)
  }

  createProposal = () => {}
  approveProposal = () => {}
  executeProposal = () => {}

  execute = async () => {
    const rawTx = await this.command.makeRawTransaction()
    console.log(rawTx)
    return {} as Result<TransactionResponse>
  }
}
