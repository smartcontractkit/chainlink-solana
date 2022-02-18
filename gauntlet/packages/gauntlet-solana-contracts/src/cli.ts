import { executeCLI } from '@chainlink/gauntlet-core'
import { existsSync } from 'fs'
import path from 'path'
import { io } from '@chainlink/gauntlet-core/dist/utils'
import { commands } from '.'
;(async () => {
  try {
    const networkPossiblePaths = ['./networks', './packages/gauntlet-solana-contracts/networks']
    const networkPath = networkPossiblePaths.filter((networkPath) =>
      existsSync(path.join(process.cwd(), networkPath)),
    )[0]
    const result = await executeCLI(commands, networkPath)
    if (result) {
			io.saveJSON(result, process.env['REPORT_NAME'] ? process.env['REPORT_NAME'] : 'report')
		}
  } catch (e) {
    console.log(e)
    console.log('Solana Command execution error', e.message)
    process.exitCode = 1
  }
})()
