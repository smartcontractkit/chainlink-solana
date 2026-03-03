import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class Initialize extends SolanaCommand {
  static id = 'access_controller:initialize'
  static category = CONTRACT_LIST.ACCESS_CONTROLLER

  static examples = ['yarn gauntlet access_controller:initialize --network=devnet']

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const accessController = getContract(CONTRACT_LIST.ACCESS_CONTROLLER, '')
    const address = accessController.programId.toString()
    const program = this.loadProgram(accessController.idl, address)

    const state = Keypair.generate()

    logger.loading(`Initializing access controller contract with State at ${state.publicKey}...`)

    const createStateIx = await program.account.accessController.createInstruction(state)
    const initIx = await program.methods
      .initialize()
      .accounts({
        state: state.publicKey,
        owner: this.wallet.publicKey,
      })
      .instruction()

    const txHash = await this.sendTxWithIDL(this.signAndSendRawTx, accessController.idl)(
      [createStateIx, initIx],
      [state],
    )

    logger.success(`TX ${txHash}`)

    logger.info(`
    STATE ACCOUNTS:
      - State: ${state.publicKey}
      - Owner: ${this.wallet.publicKey}
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
