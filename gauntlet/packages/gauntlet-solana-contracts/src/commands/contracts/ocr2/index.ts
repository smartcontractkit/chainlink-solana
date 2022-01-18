import Initialize from './initialize'
import OCR2InitializeFlow from './initialize.flow'
import BeginOffchainConfig from './offchainConfig/begin'
import BeginOffchainConfigMultisig from './offchainConfig/beginMultisig'
import CommitOffchainConfig from './offchainConfig/commit'
import SetOffchainConfigFlow from './offchainConfig/setOffchainConfig.flow'
import WriteOffchainConfig from './offchainConfig/write'
import PayRemaining from './payRemaining'
import ReadState from './read'
import SetBillingAccessController from './setBillingAccessController'
import SetRequesterAccessController from './setRequesterAccessController'
import SetBilling from './setBilling'
import SetConfig from './setConfig'
import SetConfigMultisig from './setConfigMultisig'
import SetPayees from './setPayees'
import SetPayeesMultisig from './setPayeesMultisig'
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
  BeginOffchainConfigMultisig,
  WriteOffchainConfig,
  CommitOffchainConfig,
  SetBillingAccessController,
  SetRequesterAccessController,
  SetConfigMultisig,
  SetPayeesMultisig,
  // Inspection
  ...Inspection,
  // ONLY DEV
  Transmit,
  SetupFlow,
  SetupRDDFlow,
]
