import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetBilling extends SolanaCommand {
  static id = 'ocr2:set_billing'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_billing --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --payment=1',
    'yarn gauntlet ocr2:set_billing --network=local --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR --payment=1 EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
    this.requireFlag('payment', 'Provide a observation payment amount')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)

    const info = await program.account.state.fetch(state)
    const billingAC = new PublicKey(info.config.billingAccessController)

    logger.loading('Setting billing...')
    const tx = await program.rpc.setBilling(new BN(this.flags.payment), {
      accounts: {
        state: state,
        authority: this.wallet.payer.publicKey,
        accessController: billingAC,
      },
      signers: [this.wallet.payer],
    })

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
