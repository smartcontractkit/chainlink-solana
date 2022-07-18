import { SendRawTx } from '@chainlink/solana-gauntlet'
import { wrapCommand, multisigCommands } from '@chainlink/gauntlet-serum-multisig'
import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'

const baseList = [...Solana, SendRawTx]

export const commands = {
  custom: [...multisigCommands, ...baseList, ...baseList.map(wrapCommand)],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
