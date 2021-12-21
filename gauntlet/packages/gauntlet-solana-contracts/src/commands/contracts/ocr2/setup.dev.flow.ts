import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { logger } from '@chainlink/gauntlet-core/dist/utils'

import { CONTRACT_ENV_NAMES, CONTRACT_LIST, getDeploymentContract } from '../../../lib/contracts'
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
import SetBilling from './setBilling'

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
        alphaReportInfinite: false,
        alphaReportPpb: 0,
        alphaAcceptInfinite: false,
        alphaAcceptPpb: 30,
        deltaCNanoseconds: 30,
      },
      maxDurationQueryNanoseconds: 30,
      maxDurationObservationNanoseconds: 30,
      maxDurationReportNanoseconds: 30,
      maxDurationShouldAcceptFinalizedReportNanoseconds: 30,
      maxDurationShouldTransmitAcceptedReportNanoseconds: 30,
      configPublicKeys: [
        '5cd10bf991c8b0db7bee3ec371c7795a69297b6bccf7b4d738e0920b56131772',
        'd58a9b179d5ac550376734ce1da5ee4572718fd6d315e0541b1da1d1671d0d71',
        'ef104fe8812c2c73d4c1b57dc82a15f8dd5a23149bd91917abad295f305ed21a',
      ],
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
      allowFundRecipient: false,
    }

    const configInput = {
      oracles: [
        {
          transmitter: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
          signer: '0cAFF71b6Dbb4f9Ebc862F8E9C124E737C917e80',
        },
        {
          transmitter: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
          signer: '6b211EdeF015C9931eA7D65CD326472891ecf501',
        },
        {
          transmitter: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
          signer: 'C6CD7e27Ea7653362906A7C9923c15602dC04F41',
        },
        {
          transmitter: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
          signer: '1b7c57E22a4D4B6c94365A73AD5FF743DBE9c55E',
        },
      ],
      f: 1,
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
        name: 'Set Environment',
        exec: this.setEnvironment,
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
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          version: this.flags.version || 1,
        },
      },
      {
        name: 'Write Offchain Config',
        command: WriteOffchainConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          input: offchainConfigInput,
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
          input: configInput,
        },
      },
      {
        name: 'Set Payees',
        command: SetPayees,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          input: payeesInput,
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
            validator: this.getReportStepDataById(FlowCommand.ID.contract(this.stepIds.VALIDATOR)),
            threshold: 1,
          },
        },
      },
      {
        name: 'Add access to validator on AC',
        command: AddAccess,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
          address: FlowCommand.ID.data(this.stepIds.OCR_2, 'validatorAuthority'),
        },
      },
    ]
  }

  setEnvironment = async () => {
    const programsPublicKeys = await Promise.all(
      [
        CONTRACT_LIST.ACCESS_CONTROLLER,
        CONTRACT_LIST.OCR_2,
        CONTRACT_LIST.DEVIATION_FLAGGING_VALIDATOR,
      ].map(async (name) => (await getDeploymentContract(name, '')).programKeypair.publicKey.toString()),
    )
    logger.info(`
      Setting the following env variables. Include them into .env.${this.flags.network} for future runs
        ${CONTRACT_ENV_NAMES[CONTRACT_LIST.ACCESS_CONTROLLER]}=${programsPublicKeys[0]}
        ${CONTRACT_ENV_NAMES[CONTRACT_LIST.OCR_2]}=${programsPublicKeys[1]}
        ${CONTRACT_ENV_NAMES[CONTRACT_LIST.DEVIATION_FLAGGING_VALIDATOR]}=${programsPublicKeys[2]}
      `)
    process.env[CONTRACT_ENV_NAMES[CONTRACT_LIST.ACCESS_CONTROLLER]] = programsPublicKeys[0]
    process.env[CONTRACT_ENV_NAMES[CONTRACT_LIST.OCR_2]] = programsPublicKeys[1]
    process.env[CONTRACT_ENV_NAMES[CONTRACT_LIST.DEVIATION_FLAGGING_VALIDATOR]] = programsPublicKeys[2]
  }
}
