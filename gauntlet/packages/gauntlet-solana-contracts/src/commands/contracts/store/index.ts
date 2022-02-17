import Initialize from './initialize'
import CreateFeed from './createFeed'
import SetValidatorConfig from './setValidatorConfig'
import SetWriter from './setWriter'
import SetLoweringAccessController from './setLoweringAccessController'
import Migrate from './migrate'
import TransferFeedOwnership from './transferFeedOwnership'
import AcceptFeedOwnership from './acceptFeedOwnership'
import TransferStoreOwnership from './transferStoreOwnership'
import AcceptStoreOwnership from './acceptStoreOwnership'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeUpgradeProgramCommand } from '../../abstract/upgrade'
import Inspect from './inspect'

export default [
  Initialize,
  CreateFeed,
  SetValidatorConfig,
  SetWriter,
  SetLoweringAccessController,
  Inspect,
  Migrate,
  TransferFeedOwnership,
  AcceptFeedOwnership,
  TransferStoreOwnership,
  AcceptStoreOwnership,
  makeUpgradeProgramCommand(CONTRACT_LIST.STORE),
]
