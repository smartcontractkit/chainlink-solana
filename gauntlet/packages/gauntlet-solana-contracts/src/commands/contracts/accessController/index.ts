import Initialize from './initialize'
import AddAccess from './addAccess'
import ReadState from './read'
import { makeAcceptOwnershipCommand } from '../ownership/acceptOwnership'
import { makeTransferOwnershipCommand } from '../ownership/transferOwnership'
import { makeInspectOwnershipCommand } from '../ownership/inspectOwnership'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeUpgradeProgramCommand } from '../../abstract/upgrade'

const getOwner = async (program, state) => {
  const contractState = await program.account.accessController.fetch(state)
  return {
    owner: contractState?.owner,
    proposedOwner: contractState?.proposedOwner,
  }
}

export default [
  Initialize,
  AddAccess,
  ReadState,
  makeAcceptOwnershipCommand(CONTRACT_LIST.ACCESS_CONTROLLER, getOwner),
  makeTransferOwnershipCommand(CONTRACT_LIST.ACCESS_CONTROLLER, getOwner),
  makeInspectOwnershipCommand(CONTRACT_LIST.ACCESS_CONTROLLER, getOwner),
  makeUpgradeProgramCommand(CONTRACT_LIST.ACCESS_CONTROLLER),
]
