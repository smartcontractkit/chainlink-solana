import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetAccessController extends SolanaCommand {
  static id = 'store:set_lowering_access_controller'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_lowering_access_controller --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --accessController=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
    this.requireFlag('accessController', 'Provide a valid access controller address')
  }

  execute = async () => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const address = store.programId.toString()
    const program = this.loadProgram(store.idl, address)

    const state = new PublicKey(this.flags.state)
    const ac = new PublicKey(this.flags.accessController)

    // console.log(program.account.store.fetch(state))
    const info = await program.account.store.fetch(state)
    const oldAC = info.loweringAccessController

    logger.log(`Access controller information:
      - Store State: ${state.toString()}
      - Old AC: ${oldAC}
      - New AC: ${ac.toString()}
    `)

    this.require(oldAC.toString() !== ac.toString(), 'New access controller is the same as existing access controller')
    await prompt(`Continue setting lowering access controller?`)

    const tx = await program.rpc.setLoweringAccessController({
      accounts: {
        store: state,
        authority: this.wallet.payer.publicKey,
        accessController: ac,
      },
      signers: [this.wallet.payer],
    })

    logger.success(`Access controller set on tx ${tx}`)
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
