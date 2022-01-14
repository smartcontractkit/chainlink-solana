import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetRequesterAccessController extends SolanaCommand {
  static id = 'ocr2:set_requester_access_controller'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_requester_access_controller --network=local --state=[STATE_ACC] --accessController=[AC_ACC]',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
    this.requireFlag('accessController', 'Provide a valid access controller address')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const ac = new PublicKey(this.flags.accessController)

    const info = await program.account.state.fetch(state)
    const oldAC = info.config.requesterAccessController

    logger.log(`Access controller information:
      - OCR State: ${state.toString()}
      - Old AC: ${oldAC}
      - New AC: ${ac.toString()}
    `)

    this.require(oldAC.toString() !== ac.toString(), 'New access controller is the same as existing access controller')
    await prompt(`Continue setting requester access controller?`)

    const tx = await program.rpc.setRequesterAccessController({
      accounts: {
        state: state,
        authority: this.wallet.payer.publicKey,
        accessController: ac,
      },
      signers: [this.wallet.payer],
    })

    logger.success(`Requester access controller set on tx ${tx}`)
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
