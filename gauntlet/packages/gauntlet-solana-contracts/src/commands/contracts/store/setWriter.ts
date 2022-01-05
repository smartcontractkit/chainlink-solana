import { Result } from '@chainlink/gauntlet-core'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetWriter extends SolanaCommand {
  static id = 'store:set_writer'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_writer --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --threshold=1000 --feed=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const owner = this.wallet.payer

    const state = new PublicKey(this.flags.state)
    const feed = new PublicKey(this.flags.feed)
    const storeAuthority = new PublicKey(this.flags.storeAuthority)

    console.log(`Setting store writer on ${state.toString()} and ${feed.toString()}`)

    const tx = await program.rpc.setValidatorConfig(storeAuthority, {
      accounts: {
        store: state,
        feed: feed,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

    logger.success(`Set writer on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString(), { state: state.toString() }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
