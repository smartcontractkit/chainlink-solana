import Initialize from './initialize'
import OCR2InitializeFlow from './initialize.flow'
import BeginOffchainConfig from './offchainConfig/begin'
import CommitOffchainConfig from './offchainConfig/commit'
import SetOffchainConfigFlow from './offchainConfig/setOffchainConfig.flow'
import WriteOffchainConfig from './offchainConfig/write'
import PayRemaining from './payRemaining'
import ReadState from './read'
import SetBilling from './setBilling'
import SetConfig from './setConfig'
import SetConfigDeployer from './setConfig.deployer'
import SetPayees from './setPayees'
import SetupFlow from './setup.dev.flow'
import SetValidatorConfig from './setValidatorConfig'
import Transmit from './transmit.dev'

export default [
  Initialize,
  OCR2InitializeFlow,
  SetConfigDeployer,
  SetBilling,
  PayRemaining,
  SetPayees,
  SetConfig,
  SetValidatorConfig,
  ReadState,
  SetOffchainConfigFlow,
  BeginOffchainConfig,
  WriteOffchainConfig,
  CommitOffchainConfig,
  // ONLY DEV
  Transmit,
  SetupFlow,
]
