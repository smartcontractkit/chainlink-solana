import { commands } from '@chainlink/gauntlet-solana-contracts'
import { executeCLI } from '@chainlink/gauntlet-core'
import { existsSync } from 'fs'
import path from 'path'
import { io } from '@chainlink/gauntlet-core/dist/utils'
import { wrapCommand } from './commands/multisig'
import multisigSpecificCommands from './commands'

export const multisigCommands = {
  custom: commands.custom.concat(multisigSpecificCommands).map(wrapCommand),
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
