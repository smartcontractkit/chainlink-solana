import Initialize from './initialize'
import OCR2InitializeFlow from './initialize.flow'
import ReadState from './read'
import SetBillingAccessController from './setBillingAccessController'
import SetRequesterAccessController from './setRequesterAccessController'
import SetBilling from './setBilling'
import AcceptProposal from './proposal/acceptProposal'
import SetupFlow from './setup.dev.flow'
import Transmit from './transmit.dev'
import Inspection from './inspection'
import { makeAcceptOwnershipCommand } from '../ownership/acceptOwnership'
import { makeInspectOwnershipCommand } from '../ownership/inspectOwnership'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeTransferOwnershipCommand } from '../ownership/transferOwnership'
import { makeUpgradeProgramCommand } from '../../abstract/upgrade'
import Fund from './fund'
import CreateProposal from './proposal/createProposal'
import ProposeConfig from './proposeConfig'
import FinalizeProposal from './proposal/finalizeProposal'
import Close from './close'
import WithdrawFunds from './withdrawFunds'
import WithdrawPayment from './withdrawPayment'

const getOwner = async (program, state) => {
  const contractState = await program.account.state.fetch(state)
  return {
    owner: contractState?.config?.owner,
    proposedOwner: contractState?.config?.proposedOwner,
  }
}

export default [
  Initialize,
  OCR2InitializeFlow,
  SetBilling,
  AcceptProposal,
  CreateProposal,
  FinalizeProposal,
  ProposeConfig,
  ReadState,
  SetBillingAccessController,
  SetRequesterAccessController,
  Fund,
  makeAcceptOwnershipCommand(CONTRACT_LIST.OCR_2, getOwner),
  makeTransferOwnershipCommand(CONTRACT_LIST.OCR_2, getOwner),
  makeInspectOwnershipCommand(CONTRACT_LIST.OCR_2, getOwner),
  makeUpgradeProgramCommand(CONTRACT_LIST.OCR_2),
  WithdrawFunds,
  WithdrawPayment,
  // Inspection
  ...Inspection,
  // ONLY DEV
  Transmit,
  SetupFlow,
  Close,
]
