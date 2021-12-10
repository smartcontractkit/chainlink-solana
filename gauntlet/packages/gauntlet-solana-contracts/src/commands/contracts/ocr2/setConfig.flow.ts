import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import BeginOffchainConfig from './offchainConfig/begin'
import WriteOffchainConfig from './offchainConfig/write'
import CommitOffchainConfig from './offchainConfig/commit'
import { makeAbstractCommand } from '../../abstract'
import { CONTRACT_LIST } from '../../../lib/contracts'
import SetConfig from './setConfig'

export default class SetOffchainConfigFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:setconfig'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:setconfig --network=local --version=1 --rdd=[PATH_TO_RDD]']

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.flow = [
      {
        name: 'Begin Offchain Config',
        command: BeginOffchainConfig,
      },
      {
        name: 'Write Offchain Config',
        command: WriteOffchainConfig,
      },
      {
        name: 'Commit Offchain Config',
        command: CommitOffchainConfig,
      },
      {
        name: 'Set Config',
        command: SetConfig,
      },
    ]
  }
}
