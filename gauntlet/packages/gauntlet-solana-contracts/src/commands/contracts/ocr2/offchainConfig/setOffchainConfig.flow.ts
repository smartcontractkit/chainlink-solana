import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../../lib/contracts'
import { makeAbstractCommand } from '../../../abstract'
import BeginOffchainConfig from './begin'
import WriteOffchainConfig from './write'
import CommitOffchainConfig from './commit'

export default class SetOffchainConfigFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:set_offchain_config:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:set_offchain_config:flow --network=local --rdd=[PATH_TO_RDD]']

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {}

    this.flow = [
      {
        name: 'Begin Offchain Config',
        command: BeginOffchainConfig,
        flags: {
          version: 2,
        },
      },
      {
        name: 'Write Offchain Config',
        command: WriteOffchainConfig,
      },
      {
        name: 'Commit Offchain Config',
        command: CommitOffchainConfig,
      },
    ]
  }
}

const ID = {
  contract: (id: number, index = 0): string => `ID.${id}.txs.${index}.contract`,
  tx: (id: number, index = 0): string => `ID.${id}.txs.${index}.tx`,
}
