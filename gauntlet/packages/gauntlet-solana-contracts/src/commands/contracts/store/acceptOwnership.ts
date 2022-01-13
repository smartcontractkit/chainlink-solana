import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class AcceptOwnership extends SolanaCommand {
  static id = 'store:accept_ownership'
  static category = CONTRACT_LIST.STORE

  static examples = ['yarn gauntlet store:accept_ownership --network=devnet --state=[STORE_STATE]']

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
    this.require(!!this.flags.to, 'Please provide flags with "to"')
  }

  execute = async () => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const storeAddress = store.programId.toString()
    const storeProgram = this.loadProgram(store.idl, storeAddress)

    const owner = this.wallet.payer

    const storeState = new PublicKey(this.flags.state)
    const proposedOwner = new PublicKey(this.flags.to)

    await prompt(`Accepting ownership of store state (${storeState.toString()}). Continue?`)

    const tx = await storeProgram.rpc.acceptOwnership(proposedOwner, {
      accounts: {
        store: storeState,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

    logger.success(`Set writer on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, storeState.toString(), { state: storeState.toString() }),
          contract: storeState.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
