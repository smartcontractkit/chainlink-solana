import Initialize from './initialize'
import CreateFeed from './createFeed'
import SetValidatorConfig from './setValidatorConfig'
import SetWriter from './setWriter'
import SetLoweringAccessController from './setLoweringAccessController'
import { makeAcceptOwnershipCommand } from '../ownership/acceptOwnership'
import { makeTransferOwnershipCommand } from '../ownership/transferOwnership'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeUpgradeProgramCommand } from '../../abstract/upgrade'

export default [
  Initialize,
  CreateFeed,
  SetValidatorConfig,
  SetWriter,
  SetLoweringAccessController,
  makeAcceptOwnershipCommand(CONTRACT_LIST.STORE),
  makeTransferOwnershipCommand(CONTRACT_LIST.STORE),
  makeUpgradeProgramCommand(CONTRACT_LIST.STORE),
]
