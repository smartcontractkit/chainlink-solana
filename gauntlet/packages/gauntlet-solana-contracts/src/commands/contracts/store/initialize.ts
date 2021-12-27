import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class Initialize extends SolanaCommand {
  static id = 'store:initialize'
  static category = CONTRACT_LIST.STORE

  static examples = ['yarn gauntlet store:initialize --network=devnet']

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const address = store.programId.toString()
    const program = this.loadProgram(store.idl, address)

    const state = Keypair.generate()
    const accessController = new PublicKey(this.flags.accessController)
    const owner = this.wallet.payer

    console.log(`Initializing store contract with State at ${state.publicKey}...`)
    const txHash = await program.rpc.initialize({
      accounts: {
        state: state.publicKey,
        owner: owner.publicKey,
        raisingAccessController: accessController,
        loweringAccessController: accessController,
      },
      signers: [state],
      instructions: [await program.account.store.createInstruction(state)],
    })

    console.log('TX', txHash)

    console.log(`
    STATE ACCOUNTS:
      - State: ${state.publicKey}
      - Payer: ${this.provider.wallet.publicKey}
      - Owner: ${owner.publicKey}
    `)

    return {
      responses: [
        {
          tx: this.wrapResponse(txHash, address!, { state: state.publicKey.toString() }),
          contract: state.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
