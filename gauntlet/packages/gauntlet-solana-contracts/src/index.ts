import { SendRawTx } from '@chainlink/gauntlet-solana'
import Solana from './commands'
import { makeAbstractCommand } from './commands/abstract'
import { defaultFlags } from './lib/args'
export { CONTRACT_LIST, getContract } from './lib/contracts'
export { makeTx } from './lib/utils'
import { wrapCommand } from './commands/contracts/multisig/multisig'
import CreateMultisig from './commands/contracts/multisig/create'
import MultisigInspect from './commands/contracts/multisig/inspect'
import SetOwners from './commands/contracts/multisig/setOwners'
import SetThreshold from './commands/contracts/multisig/setThreshold'

const baseList = [...Solana, SendRawTx]

export const commands = {
  custom: [
    ...baseList,
    ...baseList.concat([SetOwners, SetThreshold]).map(wrapCommand),
    CreateMultisig,
    MultisigInspect,
  ],
  loadDefaultFlags: () => defaultFlags,
  abstract: {
    findPolymorphic: () => undefined,
    makeCommand: makeAbstractCommand,
  },
}
