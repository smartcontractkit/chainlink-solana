import { Result } from '@chainlink/gauntlet-core'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

type Input = {
  threshold: number | string
  feed: string
}

export default class SetValidatorConfig extends SolanaCommand {
  static id = 'store:set_validator_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet store:set_validator_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --threshold=1000 --feed=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    // Can this come from rdd?
    // const rdd = getRDD(this.flags.rdd)
    return {
      threshold: this.flags.threshold,
      feed: this.flags.feed,
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

    const state = new PublicKey(this.flags.state)
    const threshold = new BN(input.threshold)
    const feed = new PublicKey(input.feed)

    console.log(`Setting store config on ${state.toString()}...`)

    const tx = await program.rpc.setValidatorConfig(threshold, {
      accounts: {
        state,
        store: feed,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

    logger.success(`Validator config on tx ${tx}`)

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
