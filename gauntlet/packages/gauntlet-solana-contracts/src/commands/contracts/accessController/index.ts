import Initialize from './initialize'
import AddAccess from './addAccess'
import ReadState from './read'
import { makeAcceptOwnershipCommand } from '../ownership/acceptOwnership'
import { makeTransferOwnershipCommand } from '../ownership/transferOwnership'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeUpgradeProgramCommand } from '../../abstract/upgrade'

export default [
  Initialize,
  AddAccess,
  ReadState,
  makeAcceptOwnershipCommand(CONTRACT_LIST.ACCESS_CONTROLLER),
  makeTransferOwnershipCommand(CONTRACT_LIST.ACCESS_CONTROLLER),
  makeUpgradeProgramCommand(CONTRACT_LIST.ACCESS_CONTROLLER),
]
