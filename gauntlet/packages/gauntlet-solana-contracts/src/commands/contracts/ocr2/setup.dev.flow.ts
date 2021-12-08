import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeAbstractCommand } from '../../abstract'
import Initialize from './initialize'
import InitializeAC from '../accessController/initialize'
import InitializeValidator from '../validator/initialize'
import DeployToken from '../token/deploy'
import SetPayees from './setPayees'
import SetValidatorConfig from './setValidatorConfig'
import AddAccess from '../accessController/addAccess'
import BeginOffchainConfig from './offchainConfig/begin'
import WriteOffchainConfig from './offchainConfig/write'
import CommitOffchainConfig from './offchainConfig/commit'
import SetConfig from './setConfig'

// TODO: Remove. Useful for dev testing
export default class SetupFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:setup:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:setup:flow --network=local --version=1']

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      BILLING_ACCESS_CONTROLLER: 1,
      REQUEST_ACCESS_CONTROLLER: 2,
      OCR_2: 3,
      TOKEN: 4,
      VALIDATOR: 5,
    }

    const offchainConfigInput = {
      deltaProgressNanoseconds: 30,
      deltaResendNanoseconds: 30,
      deltaRoundNanoseconds: 30,
      deltaGraceNanoseconds: 30,
      deltaStageNanoseconds: 30,
      rMax: 30,
      s: [1, 1, 1, 1],
      offchainPublicKeys: [
        '5cd10bf991c8b0db7bee3ec371c7795a69297b6bccf7b4d738e0920b56131772',
        'd58a9b179d5ac550376734ce1da5ee4572718fd6d315e0541b1da1d1671d0d71',
        'ef104fe8812c2c73d4c1b57dc82a15f8dd5a23149bd91917abad295f305ed21a',
      ],
      peerIds: [
        'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
        '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
        '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
      ],
      reportingPluginConfig: {
        alphaReportInfinite: true,
        alphaReportPpb: 30,
        alphaAcceptInfinite: true,
        alphaAcceptPpb: 30,
        deltaCNanoseconds: 30,
      },
      maxDurationQueryNanoseconds: 30,
      maxDurationObservationNanoseconds: 30,
      maxDurationReportNanoseconds: 30,
      maxDurationShouldAcceptFinalizedReportNanoseconds: 30,
      maxDurationShouldTransmitAcceptedReportNanoseconds: 30,
    }

    const payeesInput = {
      operators: [
        {
          transmitter: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
          payee: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
        },
        {
          transmitter: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
          payee: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
        },
        {
          transmitter: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
          payee: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
        },
        {
          transmitter: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
          payee: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
        },
      ],
    }

    const configInput = {
      oracles: [
        {
          transmitter: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
          signer: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
        },
        {
          transmitter: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
          signer: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
        },
        {
          transmitter: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
          signer: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
        },
        {
          transmitter: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
          signer: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
        },
      ],
      threshold: 1,
    }

    this.flow = [
      {
        name: 'Deploy AC',
        command: 'access_controller:deploy',
      },
      {
        name: 'Deploy OCR',
        command: 'ocr2:deploy',
      },
      {
        name: 'Deploy Validator',
        command: 'deviation_flagging_validator:deploy',
      },
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
        id: this.stepIds.VALIDATOR,
        flags: {
          accessController: ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
        },
      },
      {
        name: 'Initialize OCR 2',
        command: Initialize,
        flags: {
          billingAccessController: ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
          requesterAccessController: ID.contract(this.stepIds.REQUEST_ACCESS_CONTROLLER),
          link: ID.contract(this.stepIds.TOKEN),
          input: {
            minAnswer: 0,
            maxAnswer: 1000000000,
            decimals: 9,
            description: 'TEST',
          },
        },
        id: this.stepIds.OCR_2,
      },
      {
        name: 'Begin Offchain Config',
        command: BeginOffchainConfig,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          version: this.flags.version || 1,
        },
      },
      {
        name: 'Write Offchain Config',
        command: WriteOffchainConfig,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          input: offchainConfigInput,
        },
      },
      {
        name: 'Commit Offchain Config',
        command: CommitOffchainConfig,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
        },
      },
      {
        name: 'Set Config',
        command: SetConfig,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          link: ID.contract(this.stepIds.TOKEN),
          input: configInput,
        },
      },
      {
        name: 'Set Payees',
        command: SetPayees,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          input: payeesInput,
          link: ID.contract(this.stepIds.TOKEN),
        },
      },
      {
        name: 'Set Validator Config',
        command: SetValidatorConfig,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          input: {
            validator: this.getReportStepDataById(ID.contract(this.stepIds.VALIDATOR)),
            threshold: 1,
          },
        },
      },
      {
        name: 'Add access to validator on AC',
        command: AddAccess,
        flags: {
          state: ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
          address: ID.data(this.stepIds.OCR_2, 'validatorAuthority'),
        },
      },
    ]
  }
}

const ID = {
  contract: (id: number, index = 0): string => `ID.${id}.txs.${index}.contract`,
  tx: (id: number, index = 0): string => `ID.${id}.txs.${index}.tx`,
  data: (id: number, key = ''): string => `ID.${id}.data.${key}`,
}
