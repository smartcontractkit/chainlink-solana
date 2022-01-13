import { Result } from '@chainlink/gauntlet-core'
import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class TransferWwnership extends SolanaCommand {
  static id = 'store:transfer_ownership'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:transfer_ownership --network=devnet --state=[STORE_STATE] --to=[PROPOSED_OWNER]',
  ]

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

    await prompt(
      `Transfering ownership of store state (${storeState.toString()}) to: (${proposedOwner.toString()}). Continue?`,
    )

    const tx = await storeProgram.rpc.transferOwnership(proposedOwner, {
      accounts: {
        store: storeState,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

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
