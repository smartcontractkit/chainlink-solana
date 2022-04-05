import { executeCLI } from '@chainlink/gauntlet-core'
import { existsSync } from 'fs'
import path from 'path'
import { io } from '@chainlink/gauntlet-core/dist/utils'
import { wrapCommand } from './commands/multisig'
import multisigSpecificCommands from './commands'
import CreateMultisig from './commands/create'
import MultisigInspect from './commands/inspect'

export const multisigCommands = {
  custom: [multisigSpecificCommands.map(wrapCommand), CreateMultisig, MultisigInspect],
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
    if (result) {
      io.saveJSON(result, process.env['REPORT_NAME'] ? process.env['REPORT_NAME'] : 'report')
    }
  } catch (e) {
    console.log(e)
    console.log('Solana Command execution error', e.message)
  }
})()
