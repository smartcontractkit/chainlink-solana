import { executeCLI } from '@chainlink/gauntlet-core'
import { existsSync } from 'fs'
import path from 'path'
import { io } from '@chainlink/gauntlet-core/dist/utils'
import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'
import { SolanaCommand } from '@chainlink/gauntlet-solana'

export const commands = {
  custom: [...Solana] as any[],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
// TODO: This should be at cli.ts
// ;(async () => {
//   try {
//     const networkPossiblePaths = ['./networks', './packages/gauntlet-solana-contracts/networks']
//     const networkPath = networkPossiblePaths.filter((networkPath) =>
//       existsSync(path.join(process.cwd(), networkPath)),
//     )[0]
//     const result = await executeCLI(commands, networkPath)
//     io.saveJSON(result, 'report')
//   } catch (e) {
//     console.log(e)
//     console.log('Solana Command execution error', e.message)
//   }
// })()
