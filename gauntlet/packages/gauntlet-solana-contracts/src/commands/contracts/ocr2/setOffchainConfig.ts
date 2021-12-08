import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../lib/contracts'

export default class SetOffchainConfig extends SolanaCommand {
  static id = 'ocr2:set_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_offchain_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:set_offchain_config --network=devnet --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]
  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    logger.info('Command to be defined')
    return {
      responses: [
        {
          tx: { ...this.wrapResponse('tx', ''), wait: async (a) => ({ success: true }) },
          contract: '',
        },
      ],
    } as Result<TransactionResponse>
  }
}
