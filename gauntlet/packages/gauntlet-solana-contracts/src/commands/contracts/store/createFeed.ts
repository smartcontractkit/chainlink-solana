import { Result } from '@chainlink/gauntlet-core'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  store: string
  granularity: number
  liveLength: number,
}

export default class CreateFeed extends SolanaCommand {
  static id = 'store:create_feed'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet store:create_feed --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --granularity=30 --live-length 86400 --store=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
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
    const store = getContract(CONTRACT_LIST.STORE, '')
    const address = store.programId.toString()
    const program = this.loadProgram(store.idl, address)

    const state = new PublicKey(this.flags.state)
    const feed = Keypair.generate()
    const input = this.makeInput(this.flags.input)
    const owner = this.wallet.payer

    console.log('INPUT', input)

    const store = new PublicKey(input.store)

    console.log(`Creating feed...`)
    
    // TODO: assert length >= liveLength
    
    const tx = await program.rpc.createFeed(granularity, liveLength, {
      accounts: {
        state: state,
        store: store,
        authority: owner.publicKey,
      },
      signers: [owner, feed],
      preInstructions: [
        await program.account.transmissions.createInstruction(feed, 8+128+length*24),
      ],
    })

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
