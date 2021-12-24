import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeAbstractCommand } from '../../abstract'
import Initialize from './initialize'
import SetPayees from './setPayees'
import BeginOffchainConfig from './offchainConfig/begin'
import WriteOffchainConfig from './offchainConfig/write'
import CommitOffchainConfig from './offchainConfig/commit'
import SetConfig from './setConfig'
import SetBilling from './setBilling'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
export default class OCR2InitializeFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:initialize:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:initialize:flow --network=local --id=[ID] --rdd=[PATH_TO_RDD] --version=[OFFCHAIN_CONFIG_VERSION(optional)]',
  ]

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      OCR_2: 1,
    }

    this.flow = [
      {
        name: 'Initialize OCR 2',
        command: Initialize,
        flags: {
          billingAccessController: process.env.BILLING_ACCESS_CONTROLLER || this.flags.billingAccessController,
          requesterAccessController: process.env.REQUESTER_ACCESS_CONTROLLER || this.flags.requesterAccessController,
          link: process.env.LINK || this.flags.link,
        },
        id: this.stepIds.OCR_2,
      },
      {
        name: 'Change RDD',
        exec: this.showRddInstructions,
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
          link: process.env.LINK || this.flags.link,
        },
      },
      {
        name: 'Set Billing',
        command: SetBilling,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
        },
      },
    ]
  }

  showRddInstructions = async () => {
    logger.info(
      `
        Change the RDD ID with the new state address: 
          - State Address: ${this.getReportStepDataById(FlowCommand.ID.contract(this.stepIds.OCR_2))}
        Include the transmissions account into the contract information:
          - Transmissions Address: ${this.getReportStepDataById(
            FlowCommand.ID.data(this.stepIds.OCR_2, 'transmissions'),
          )}
      `,
    )

    await prompt('Ready? Continue')
  }
}
