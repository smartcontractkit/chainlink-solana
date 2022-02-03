import { SendRawTx } from '@chainlink/gauntlet-solana'
import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'
export { CONTRACT_LIST, getContract } from './lib/contracts'
export { makeTx } from './lib/utils'

export const commands = {
  custom: [...Solana, SendRawTx],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
