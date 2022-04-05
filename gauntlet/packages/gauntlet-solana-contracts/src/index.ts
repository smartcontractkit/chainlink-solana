import { SendRawTx } from '@chainlink/gauntlet-solana'
import { wrapCommand } from '@chainlink/gauntlet-serum-multisig/src/commands/multisig'
import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'
export { CONTRACT_LIST, getContract } from './lib/contracts'
export { makeTx } from './lib/utils'

const baseList = [...Solana, SendRawTx]

export const commands = {
  custom: [...baseList, ...baseList.map(wrapCommand)],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
