import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetLoweringAccessController extends SolanaCommand {
  static id = 'store:set_lowering_access_controller'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_lowering_access_controller --network=devnet --accessController=[AC_ACC] [AGGREGATOR_ADDRESS]',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('accessController', 'Provide a valid access controller address')
  }

  execute = async () => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const address = store.programId.toString()
    const program = this.loadProgram(store.idl, address)

    const state = new PublicKey(this.args[0])
    const ac = new PublicKey(this.flags.accessController)

    const info = await program.account.store.fetch(state) as any
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
