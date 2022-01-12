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
import OCR2Inspect from './inspection/inspect'
import CreateFeed from '../store/createFeed'
import SetWriter from '../store/setWriter'

export default class OCR2InitializeFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:initialize:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:initialize:flow --network=local --id=[ID] --rdd=[PATH_TO_RDD]']

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      FEED: 1,
      OCR_2: 2,
    }

    this.requireFlag('rdd', 'This flow only works with information coming from RDD. Please provide the --rdd flag')

    this.flow = [
      {
        name: 'Create Transmissions/Feed',
        command: CreateFeed,
        id: this.stepIds.FEED,
      },
      {
        name: 'Update RDD aggregator information',
        exec: this.showRddTransmissionInstructions,
      },
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
        name: 'Change RDD Identifier',
        exec: this.showRddIdentifierInstructions,
      },
      {
        name: 'Set writer on Store',
        command: SetWriter,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
        },
      },
      {
        name: 'Begin Offchain Config',
        command: BeginOffchainConfig,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          version: 2,
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
      {
        name: 'Inspection',
        command: OCR2Inspect,
        flags: {
          state: FlowCommand.ID.contract(this.stepIds.OCR_2),
          billingAccessController: process.env.BILLING_ACCESS_CONTROLLER || this.flags.billingAccessController,
          requesterAccessController: process.env.REQUESTER_ACCESS_CONTROLLER || this.flags.requesterAccessController,
          link: process.env.LINK || this.flags.link,
        },
      },
    ]
  }

  showRddIdentifierInstructions = async () => {
    logger.info(
      `
        Change the RDD ID with the new state address: 
          - State Address: ${this.getReportStepDataById(FlowCommand.ID.contract(this.stepIds.OCR_2))}
      `,
    )

    await prompt('Ready? Continue')
  }

  showRddTransmissionInstructions = async () => {
    logger.info(
      `
        Update the RDD Aggregator information with the new transmissions address: 
          - Transmissions address: ${this.getReportStepDataById(
            FlowCommand.ID.data(this.stepIds.FEED, 'transmissions'),
          )}
      `,
    )

    await prompt('Ready? Continue')
  }
}
