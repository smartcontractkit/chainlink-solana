import { wrapCommand } from './commands/multisig'
import multisigSpecificCommands from './commands'
import CreateMultisig from './commands/create'
import MultisigInspect from './commands/inspect'

const multisigCommands = [...multisigSpecificCommands.map(wrapCommand), CreateMultisig, MultisigInspect]

export { multisigCommands, wrapCommand }
