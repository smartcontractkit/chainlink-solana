import { Result } from '@chainlink/gauntlet-core'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  store: string
  granularity: number
  liveLength: number
}

export default class CreateFeed extends SolanaCommand {
  static id = 'store:create_feed'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:create_feed --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --granularity=30 --liveLength 86400 --store=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const aggregator = rdd.contracts[this.flags.state]
    return {
      store: aggregator.store,
      granularity: this.flags.granularity,
      liveLength: this.flags.liveLength,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const input = this.makeInput(this.flags.input)
    const owner = this.wallet.payer

    const store = new PublicKey(input.store)
    const state = new PublicKey(this.flags.state)
    const feed = Keypair.generate()

    const granularity = new BN(input.granularity)
    const liveLength = new BN(input.liveLength)
    const length = new BN(8096)
    const feedAccountLength = new BN(8 + 128 + length.toNumber() * 24)

    console.log(`Creating feed...`)

    this.require(
      length.gte(liveLength),
      `Length (${length.toNumber()}) must be greater than liveLength (${liveLength.toNumber()})`,
    )

    const tx = await program.rpc.createFeed(granularity, liveLength, {
      accounts: {
        store: store,
        feed: feed.publicKey,
        authority: owner.publicKey,
      },
      signers: [owner, feed],
      instructions: [await program.account.transmissions.createInstruction(feed, feedAccountLength.toNumber())],
    })

    logger.success(`Created feed on tx ${tx}`)
    console.log(`
    STATE ACCOUNTS:
      - Store: ${store}
      - Feed/Transmissions: ${feed.publicKey}
    `)

    return {
      data: {
        state: state.toString(),
        transmissions: feed.publicKey.toString(),
      },
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString(), {
            state: state.toString(),
            transmissions: feed.publicKey.toString(),
          }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
