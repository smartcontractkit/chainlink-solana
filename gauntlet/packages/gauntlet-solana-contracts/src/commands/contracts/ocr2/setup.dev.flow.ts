import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { CONTRACT_ENV_NAMES, CONTRACT_LIST, getDeploymentContract } from '../../../lib/contracts'
import { makeAbstractCommand } from '../../abstract'
import Initialize from './initialize'
import InitializeAC from '../accessController/initialize'
import InitializeStore from '../store/initialize'
import DeployToken from '../token/deploy'
import SetBilling from './setBilling'
import CreateFeed from '../store/createFeed'
import SetWriter from '../store/setWriter'
import ProposeConfig from './proposeConfig'
import AcceptProposal from './proposal/acceptProposal'
import CreateAccount from '../token/createAccount'
import { getAssociatedTokenAddress } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'

// TODO: Remove. Useful for dev testing
export default class SetupFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:setup:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:setup:flow --network=local --version=1']
  configInput: any

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      BILLING_ACCESS_CONTROLLER: 1,
      REQUEST_ACCESS_CONTROLLER: 2,
      OCR_2: 3,
      TOKEN: 4,
      STORE: 5,
      FEED: 6,
      PROPOSAL: 7,
    }

    const randomSecret = 'awe fluke polygon tonic lilly acuity onyx debra bound gilbert wane'

    const offchainConfigInput = {
      deltaProgressNanoseconds: 300000000,
      deltaResendNanoseconds: 300000000,
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

    const _toHex = (a: string) => Buffer.from(a, 'hex')
    this.configInput = {
      oracles: [
        {
          transmitter: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
          signer: '0cAFF71b6Dbb4f9Ebc862F8E9C124E737C917e80',
          payee: 'DxRwKpwNBuMzKf5YEG1vLpnRbWeKo1Z4tKHfFGt8vUkj',
        },
        {
          transmitter: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
          signer: '6b211EdeF015C9931eA7D65CD326472891ecf501',
          payee: '8sdUrh9LQdAXhrgFEBDxnUauJTTLfEq5PNsJbn9Pw19K',
        },
        {
          transmitter: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
          signer: 'C6CD7e27Ea7653362906A7C9923c15602dC04F41',
          payee: '9n1sSGA5rhfsQyaX3tHz3ZU1ffR6V8KffvWtFPBcFrJw',
        },
        {
          transmitter: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
          signer: '1b7c57E22a4D4B6c94365A73AD5FF743DBE9c55E',
          payee: 'G5LdWMvWoQQ787iPgWbCSTrkPB5Li9e2CWi6jYuAUHUH',
        },
      ].sort((a, b) => Buffer.compare(_toHex(a.signer), _toHex(b.signer))),
      f: 1,
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
      //   name: 'Deploy Store',
      //   command: 'store:deploy',
      // },
      {
        name: 'Deploy LINK',
        command: DeployToken,
        id: this.stepIds.TOKEN,
      },
      {
        name: 'Create payee accounts',
        command: CreateAccount,
        id: this.stepIds.ASSOCIATED_TOKEN_ACCOUNT,
        flags: {
          link: FlowCommand.ID.contract(this.stepIds.TOKEN),
        },
        args: this.configInput.oracles.map((oracle) => oracle.transmitter.toString()),
      },
      {
        name: 'Update payees',
        exec: this.updatePayees,
      },
      // {
      //   name: 'Set Environment',
      //   exec: this.setEnvironment,
      // },
      // Constant Contracts
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
        name: 'Initialize Store',
        command: InitializeStore,
        id: this.stepIds.STORE,
        flags: {
          accessController: FlowCommand.ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
        },
      },
      // Feed deployment. Unique
      {
        name: 'Create Feed',
        command: CreateFeed,
        id: this.stepIds.FEED,
        flags: {
          input: {
            store: FlowCommand.ID.contract(this.stepIds.STORE),
            granularity: 30,
            liveLength: 400,
            decimals: 9,
            description: 'TEST',
          },
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
            transmissions: FlowCommand.ID.data(this.stepIds.FEED, 'transmissions'),
          },
        },
        id: this.stepIds.OCR_2,
      },
      {
        name: 'Set writer on Store',
        command: SetWriter,
        flags: {
          input: {
            transmissions: FlowCommand.ID.data(this.stepIds.FEED, 'transmissions'),
            store: FlowCommand.ID.contract(this.stepIds.STORE),
          },
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Set Billing',
        command: SetBilling,
        flags: {
          link: FlowCommand.ID.contract(this.stepIds.TOKEN),
          input: {
            observationPaymentGjuels: '1',
            transmissionPaymentGjuels: '1',
          },
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Propose Config',
        command: ProposeConfig,
        id: this.stepIds.PROPOSAL,
        flags: {
          link: FlowCommand.ID.contract(this.stepIds.TOKEN),
          input: {
            oracles: this.configInput.oracles,
            f: this.configInput.f,
            offchainConfig: offchainConfigInput,
          },
          secret: randomSecret,
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Accept Proposal',
        command: AcceptProposal,
        flags: {
          link: FlowCommand.ID.contract(this.stepIds.TOKEN),
          input: {
            proposalId: FlowCommand.ID.data(this.stepIds.PROPOSAL, 'proposal'),
            secret: randomSecret,
            version: 2,
            f: this.configInput.f,
            tokenMint: FlowCommand.ID.contract(this.stepIds.TOKEN),
            oracles: this.configInput.oracles,
            offchainConfig: offchainConfigInput,
          },
          proposalId: FlowCommand.ID.data(this.stepIds.PROPOSAL, 'proposal'),
          secret: randomSecret,
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
    ]
  }

  setEnvironment = async () => {
    const programsPublicKeys = await Promise.all(
      [CONTRACT_LIST.ACCESS_CONTROLLER, CONTRACT_LIST.OCR_2, CONTRACT_LIST.STORE].map(async (name) =>
        getDeploymentContract(name, '').programKeypair.publicKey.toString(),
      ),
    )
    logger.info(`
      Setting the following env variables. Include them into .env.${this.flags.network} for future runs
        ${CONTRACT_ENV_NAMES[CONTRACT_LIST.ACCESS_CONTROLLER]}=${programsPublicKeys[0]}
        ${CONTRACT_ENV_NAMES[CONTRACT_LIST.OCR_2]}=${programsPublicKeys[1]}
        ${CONTRACT_ENV_NAMES[CONTRACT_LIST.STORE]}=${programsPublicKeys[2]}
      `)
    process.env[CONTRACT_ENV_NAMES[CONTRACT_LIST.ACCESS_CONTROLLER]] = programsPublicKeys[0]
    process.env[CONTRACT_ENV_NAMES[CONTRACT_LIST.OCR_2]] = programsPublicKeys[1]
    process.env[CONTRACT_ENV_NAMES[CONTRACT_LIST.STORE]] = programsPublicKeys[2]
  }

  updatePayees = async () => {
    // payees have to be derived token addresses, matching the ones we just
    // created in the previous step.
    for (let oracle of this.configInput.oracles) {
      oracle.payee = (
        await getAssociatedTokenAddress(
          new PublicKey(this.getReportStepDataById(FlowCommand.ID.contract(this.stepIds.TOKEN))),
          new PublicKey(oracle.transmitter),
          true,
        )
      ).toString()
    }
  }
}
