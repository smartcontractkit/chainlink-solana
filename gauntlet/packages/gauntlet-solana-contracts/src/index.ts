import { SendRawTx } from '@chainlink/gauntlet-solana'
import { wrapCommand, multisigCommands } from '@chainlink/gauntlet-serum-multisig'
import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'

const baseList = [...Solana, SendRawTx]

export const commands = {
  custom: [
    ...multisigCommands,
    ...baseList,
    ...baseList.map(wrapCommand)
  ],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
