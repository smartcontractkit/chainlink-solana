import { Result } from '@chainlink/gauntlet-core'
import { TransactionResponse, SolanaCommand, RawTransaction } from '@chainlink/gauntlet-solana'

enum ACTIONS {
  create = 'create',
  approve = 'approve',
  execute = 'execute',
}

export const wrapCommand = (command) => {
  return class Multisig extends SolanaCommand {
    command: SolanaCommand

    static id = `${command.id}`

    constructor(flags, args) {
      super(flags, args)

      this.command = new command(flags, args)
      this.command.invokeMiddlewares(this.command, this.command.middlewares)
    }

    createProposal = (tx: RawTransaction[]) => {
      return {} as Result<TransactionResponse>
    }
    approveProposal = (tx: RawTransaction[]) => {
      return {} as Result<TransactionResponse>
    }
    executeProposal = (tx: RawTransaction[]) => {
      return {} as Result<TransactionResponse>
    }

    execute = async () => {
      const rawTx: RawTransaction[] = await this.command.makeRawTransaction()

      const actions = {
        [ACTIONS.create]: this.createProposal,
        [ACTIONS.approve]: this.approveProposal,
        [ACTIONS.execute]: this.executeProposal,
      }

      return actions[this.flags.action](rawTx)
    }
  }
}
