import { Result } from '@chainlink/gauntlet-core'
import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetBilling extends SolanaCommand {
  static id = 'ocr2:close'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:close --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC']

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)

    await prompt(`Continue closing ocr2 feed with address ${state.toString()}?`)

    const tx = await await program.rpc.close({
      accounts: {
        state: state,
        receiver: this.provider.wallet.publicKey,
        authority: this.provider.wallet.publicKey,
      },
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
