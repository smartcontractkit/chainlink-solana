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
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeTransferOwnershipCommand } from '../ownership/transferOwnership'
import { makeUpgradeProgramCommand } from '../../abstract/upgrade'
import Fund from './fund'

export default [
  Initialize,
  OCR2InitializeFlow,
  SetBilling,
  AcceptProposal,
  ReadState,
  SetBillingAccessController,
  SetRequesterAccessController,
  // Inspection
  ...Inspection,
  // ONLY DEV
  Transmit,
  SetupFlow,
  Fund,
  makeAcceptOwnershipCommand(CONTRACT_LIST.OCR_2),
  makeTransferOwnershipCommand(CONTRACT_LIST.OCR_2),
  makeUpgradeProgramCommand(CONTRACT_LIST.OCR_2),
]
