import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  store: string
  granularity: number
  liveLength: number
  decimals: number | string
  description: string
}

export default class CreateFeed extends SolanaCommand {
  static id = 'store:create_feed'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:create_feed --network=devnet --store=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const aggregator = rdd.contracts[this.flags.id]
    return {
      store: aggregator.store,
      granularity: aggregator.granularity,
      liveLength: aggregator.liveLength,
      decimals: aggregator.decimals,
      description: aggregator.name,
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const input = this.makeInput(this.flags.input)
    const owner = this.wallet.payer

    const store = new PublicKey(input.store)
    const feed = Keypair.generate()

    const granularity = new BN(input.granularity)
    const liveLength = new BN(input.liveLength)
    const length = new BN(this.flags.length || 8096)
    const feedAccountLength = new BN(8 + 128 + length.toNumber() * 48)
    const decimals = new BN(input.decimals)
    const description = input.description || ''

    this.require(
      feedAccountLength.gte(liveLength),
      `Feed account Length (${feedAccountLength.toNumber()}) must be greater than liveLength (${liveLength.toNumber()})`,
    )

    logger.info(`
      - Decimals: ${decimals}
      - Description: ${description}
      - Live Length: ${liveLength.toNumber()}
      - Granularity (historical): ${granularity.toNumber()}
      - Historical Length: ${length.toNumber() - liveLength.toNumber()}
      - Total Length: ${length.toNumber()}
    `)

    await prompt('Continue creating new OCR 2 feed?')
    logger.loading(`Creating feed...`)

    const tx = await program.rpc.createFeed(description, decimals, granularity, liveLength, {
      accounts: {
        store: store,
        feed: feed.publicKey,
        authority: owner.publicKey,
      },
      signers: [owner, feed],
      instructions: [await program.account.transmissions.createInstruction(feed, feedAccountLength.toNumber())],
    })

    logger.success(`Created feed on tx ${tx}`)
    logger.info(`
      STATE ACCOUNTS:
        - Store: ${store}
        - Feed/Transmissions: ${feed.publicKey}
    `)

    return {
      data: {
        transmissions: feed.publicKey.toString(),
      },
      responses: [
        {
          tx: this.wrapResponse(tx, feed.publicKey.toString(), {
            state: feed.toString(),
            transmissions: feed.publicKey.toString(),
          }),
          contract: feed.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
