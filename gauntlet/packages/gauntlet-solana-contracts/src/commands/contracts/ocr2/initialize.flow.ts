import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeAbstractCommand } from '../../abstract'
import Initialize from './initialize'
import SetBilling from './setBilling'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import OCR2Inspect from './inspection/inspect'
import CreateFeed from '../store/createFeed'
import SetWriter from '../store/setWriter'
import CreateProposal from './proposal/createProposal'
import ProposeOffchainConfig from './proposeOffchainConfig'
import ProposeConfig from './proposeConfig'
import FinalizeProposal from './proposal/finalizeProposal'
import AcceptProposal from './proposal/acceptProposal'

export default class OCR2InitializeFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:initialize:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:initialize:flow --network=local --rdd=[PATH_TO_RDD] [UNDEPLOYED_CONTRACT_ADDRESS]',
  ]

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      FEED: 1,
      OCR_2: 2,
      PROPOSAL: 3,
      PROPOSE_OFFCHAIN: 4,
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
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Set Billing',
        command: SetBilling,
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        id: this.stepIds.PROPOSAL,
        name: 'Create Proposal',
        command: CreateProposal,
      },
      {
        name: 'Propose Config',
        command: ProposeConfig,
        flags: {
          proposalId: FlowCommand.ID.data(this.stepIds.PROPOSAL, 'proposal'),
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        id: this.stepIds.PROPOSE_OFFCHAIN,
        name: 'Propose Offchain Config',
        command: ProposeOffchainConfig,
        flags: {
          proposalId: FlowCommand.ID.data(this.stepIds.PROPOSAL, 'proposal'),
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Finalize Proposal',
        command: FinalizeProposal,
        flags: {
          proposalId: FlowCommand.ID.data(this.stepIds.PROPOSAL, 'proposal'),
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Accept Proposal',
        command: AcceptProposal,
        flags: {
          proposalId: FlowCommand.ID.data(this.stepIds.PROPOSAL, 'proposal'),
          secret: FlowCommand.ID.data(this.stepIds.PROPOSE_OFFCHAIN, 'secret'),
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
      },
      {
        name: 'Inspection',
        command: OCR2Inspect,
        flags: {
          billingAccessController: process.env.BILLING_ACCESS_CONTROLLER || this.flags.billingAccessController,
          requesterAccessController: process.env.REQUESTER_ACCESS_CONTROLLER || this.flags.requesterAccessController,
          link: process.env.LINK || this.flags.link,
        },
        args: [FlowCommand.ID.contract(this.stepIds.OCR_2)],
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
