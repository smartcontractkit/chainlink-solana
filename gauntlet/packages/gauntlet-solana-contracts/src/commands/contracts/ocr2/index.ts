import Initialize from './initialize'
import OCR2InitializeFlow from './initialize.flow'
import BeginOffchainConfig from './offchainConfig/begin'
import CommitOffchainConfig from './offchainConfig/commit'
import SetOffchainConfigFlow from './offchainConfig/setOffchainConfig.flow'
import WriteOffchainConfig from './offchainConfig/write'
import ResetPendingOffchainConfig from './offchainConfig/resetPending'
import PayRemaining from './payRemaining'
import ReadState from './read'
import SetBillingAccessController from './setBillingAccessController'
import SetRequesterAccessController from './setRequesterAccessController'
import SetBilling from './setBilling'
import SetConfig from './setConfig'
import SetPayees from './setPayees'
import SetupFlow from './setup.dev.flow'
import SetupRDDFlow from './setup.dev.rdd.flow'
import Transmit from './transmit.dev'
import Inspection from './inspection'

export default [
  Initialize,
  OCR2InitializeFlow,
  SetBilling,
  PayRemaining,
  SetPayees,
  SetConfig,
  ReadState,
  SetOffchainConfigFlow,
  BeginOffchainConfig,
  WriteOffchainConfig,
  CommitOffchainConfig,
  ResetPendingOffchainConfig,
  SetBillingAccessController,
  SetRequesterAccessController,
  // Inspection
  ...Inspection,
  // ONLY DEV
  Transmit,
  SetupFlow,
  SetupRDDFlow,
]
