import { Result } from '@chainlink/gauntlet-core'
import { commands } from '@chainlink/gauntlet-solana-contracts'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { executeCLI } from '@chainlink/gauntlet-core'
import { existsSync } from 'fs'
import path from 'path'
import { io } from '@chainlink/gauntlet-core/dist/utils'

export const multisigCommands = {
  custom: commands.custom.map((command) => {
    return class Multisig extends SolanaCommand {
      command: SolanaCommand

      static id = `${command.id}`

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
  }),
  loadDefaultFlags: () => ({}),
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: () => undefined,
  },
}
;(async () => {
  try {
    const networkPossiblePaths = ['./networks', './packages/gauntlet-serum-multisig/networks']
    const networkPath = networkPossiblePaths.filter((networkPath) =>
      existsSync(path.join(process.cwd(), networkPath)),
    )[0]
    const result = await executeCLI(multisigCommands, networkPath)
    io.saveJSON(result, 'report')
  } catch (e) {
    console.log(e)
    console.log('Solana Command execution error', e.message)
  }
})()
