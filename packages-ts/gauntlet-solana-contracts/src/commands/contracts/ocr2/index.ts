import Initialize from './initialize'
import OCR2InitializeFlow from './initialize.flow'
import PayRemaining from './payRemaining'
import ReadState from './read'
import SetBilling from './setBilling'
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
  SetValidatorConfig,
  ReadState,
  // ONLY DEV
  Transmit,
  SetupFlow,
]
