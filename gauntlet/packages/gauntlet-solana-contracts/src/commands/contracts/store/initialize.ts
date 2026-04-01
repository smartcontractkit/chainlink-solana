import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { ADDITIONAL_STATE_BUFFER } from '../../../lib/constants'
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
    const defaultAccountSize = new BN(program.account.store.size).toNumber()
    const accountSize = defaultAccountSize + ADDITIONAL_STATE_BUFFER

    const accessController = new PublicKey(this.flags.accessController)
    const owner = this.wallet.publicKey

    logger.loading(`Initializing store contract with State at ${state.publicKey}...`)

    const createStateIx = await program.account.store.createInstruction(state)
    const initIx = await program.methods
      .initialize()
      .accounts({
        store: state.publicKey,
        owner: owner,
        loweringAccessController: accessController,
      })
      .instruction()

    const txHash = await this.sendTxWithIDL(this.signAndSendRawTx, store.idl)([createStateIx, initIx], [state])

    logger.success(`TX ${txHash}`)

    logger.info(`
    STATE ACCOUNTS:
      - State: ${state.publicKey}
      - Owner: ${owner}
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
