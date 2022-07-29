import { Result } from '@chainlink/gauntlet-core'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, utils } from '@chainlink/solana-gauntlet'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

type Input = {
  threshold: number | string
  feed: string
}

export default class SetValidatorConfig extends SolanaCommand {
  static id = 'store:set_validator_config'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_validator_config --network=devnet --threshold=1000 --feed=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    return {
      threshold: this.flags.threshold,
      feed: this.flags.feed,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.feed, 'Please provide flags with "feed"')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const input = this.makeInput(this.flags.input)

    const threshold = new BN(input.threshold)
    const feed = new PublicKey(input.feed)

    // Resolve the current store owner
    let feedAccount = await program.account.transmissions.fetch(feed)

    const tx = program.instruction.setValidatorConfig(threshold, {
      accounts: {
        feed: feed,
        owner: feedAccount.owner,
        authority: signer,
      },
    })

    return [tx]
  }

  execute = async () => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
    const tx = utils.makeTx(rawTx)
    logger.debug(tx)
    logger.info(`Setting validator config on ${this.flags.feed.toString()}...`)
    logger.loading('Sending tx...')
    const txhash = await this.sendTx(tx, [this.wallet.payer], contract.idl)
    logger.success(`Validator config on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.flags.feed),
          contract: this.flags.feed,
        },
      ],
    } as Result<TransactionResponse>
  }
}
