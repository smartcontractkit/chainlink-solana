import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, SystemProgram, SYSVAR_RENT_PUBKEY } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class Initialize extends SolanaCommand {
  static id = 'access_controller:initialize'
  static category = CONTRACT_LIST.ACCESS_CONTROLLER

  static examples = [
    'yarn gauntlet access_controller:initialize --network=devnet 8cMfJYzeFS2ELDSCMZK65ib9zF6DmtEqFS7sNe9dZzct',
  ]

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const accessController = getContract(CONTRACT_LIST.ACCESS_CONTROLLER, '')
    const address = accessController.programId.toString()
    const program = this.loadProgram(accessController.idl, address)

    const state = Keypair.generate()
    const owner = this.wallet.payer

    console.log(`Initializing access controller contract with State at ${state.publicKey}...`)
    const txHash = await program.rpc.initialize({
      accounts: {
        state: state.publicKey,
        owner: owner.publicKey,
      },
      signers: [owner, state],
      instructions: [await program.account.accessController.createInstruction(state)],
    })

    console.log('TX', txHash)

    console.log(`
    STATE ACCOUNTS:
      - State: ${state.publicKey}
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
