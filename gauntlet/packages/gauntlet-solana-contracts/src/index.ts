import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'
export { CONTRACT_LIST, getContract } from './lib/contracts'

export const commands = {
  custom: [...Solana],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
