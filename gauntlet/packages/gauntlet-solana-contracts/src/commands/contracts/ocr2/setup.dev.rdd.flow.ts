import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeAbstractCommand } from '../../abstract'
import Initialize from './initialize'
import InitializeAC from '../accessController/initialize'
import InitializeValidator from '../store/initialize'
import DeployToken from '../token/deploy'
import SetPayees from './setPayees'
import SetValidatorConfig from '../store/setValidatorConfig'
import AddAccess from '../accessController/addAccess'
import BeginOffchainConfig from './offchainConfig/begin'
import WriteOffchainConfig from './offchainConfig/write'
import CommitOffchainConfig from './offchainConfig/commit'
import SetConfig from './setConfig'
import SetBilling from './setBilling'

// TODO: Remove. Useful for dev testing
export default class SetupRDDFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:setup:rdd:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:setup:rdd:flow --network=local --version=1']

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      BILLING_ACCESS_CONTROLLER: 1,
      REQUEST_ACCESS_CONTROLLER: 2,
      OCR_2: 3,
      TOKEN: 4,
      STORE: 5,
    }

    this.flow = [
      // {
      //   name: 'Deploy AC',
      //   command: 'access_controller:deploy',
      // },
      // {
      //   name: 'Deploy OCR',
      //   command: 'ocr2:deploy',
      // },
      // {
      //   name: 'Deploy Validator',
      //   command: 'store:deploy',
      // },
      {
        name: 'Deploy LINK',
        command: DeployToken,
        id: this.stepIds.TOKEN,
      },
      {
        name: 'Initialize Billing AC',
        command: InitializeAC,
        id: this.stepIds.BILLING_ACCESS_CONTROLLER,
      },
      {
        name: 'Initialize Request AC',
        command: InitializeAC,
        id: this.stepIds.REQUEST_ACCESS_CONTROLLER,
      },
      {
        name: 'Initialize Validator',
        command: InitializeValidator,
        id: this.stepIds.STORE,
        flags: {
          accessController: FlowCommand.ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
        },
      },
      {
        name: 'Initialize OCR 2',
        command: Initialize,
        flags: {
          billingAccessController: FlowCommand.ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
          requesterAccessController: FlowCommand.ID.contract(this.stepIds.REQUEST_ACCESS_CONTROLLER),
          link: FlowCommand.ID.contract(this.stepIds.TOKEN),
        },
        id: this.stepIds.OCR_2,
      },
      {
        name: 'Begin Offchain Config',
        command: BeginOffchainConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          version: this.flags.version || 1,
        },
      },
      {
        name: 'Write Offchain Config',
        command: WriteOffchainConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
        },
      },
      {
        name: 'Commit Offchain Config',
        command: CommitOffchainConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
        },
      },
      {
        name: 'Set Config',
        command: SetConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
        },
      },
      {
        name: 'Set Payees',
        command: SetPayees,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          link: FlowCommand.ID.contract(this.stepIds.TOKEN),
        },
      },
      {
        name: 'Set Billing',
        command: SetBilling,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          input: {
            observationPaymentGjuels: '1',
            transmissionPaymentGjuels: '1',
          },
        },
      },
      {
        name: 'Set Validator Config',
        command: SetValidatorConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          input: {
            store: this.getReportStepDataById(FlowCommand.ID.contract(this.stepIds.STORE)),
            threshold: 1,
          },
        },
      },
      {
        name: 'Add access to store on AC',
        command: AddAccess,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
          address: FlowCommand.ID.data(this.stepIds.OCR_2, 'storeAuthority'),
        },
      },
    ]
  }
}
