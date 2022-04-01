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
import ProposeOffchainConfig from './proposeOffchainConfig'
import ProposePayees from './proposePayees'
import FinalizeProposal from './proposal/finalizeProposal'
import Close from './close'

const getOwner = async (program, state) => {
  const contractState = await program.account.state.fetch(state)
  return contractState?.config?.owner
}

export default [
  Initialize,
  OCR2InitializeFlow,
  SetBilling,
  AcceptProposal,
  CreateProposal,
  FinalizeProposal,
  ProposeConfig,
  ProposeOffchainConfig,
  ProposePayees,
  ReadState,
  SetBillingAccessController,
  SetRequesterAccessController,
  Fund,
  makeAcceptOwnershipCommand(CONTRACT_LIST.OCR_2, getOwner),
  makeTransferOwnershipCommand(CONTRACT_LIST.OCR_2, getOwner),
  makeInspectOwnershipCommand(CONTRACT_LIST.OCR_2, getOwner),
  makeUpgradeProgramCommand(CONTRACT_LIST.OCR_2),
  // Inspection
  ...Inspection,
  // ONLY DEV
  Transmit,
  SetupFlow,
  Close,
]
