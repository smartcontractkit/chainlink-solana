import { Result } from '@chainlink/gauntlet-core'
import { logger, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { makeTx } from '../../../lib/utils'

type Input = {
  threshold: number | string
  feed: string
}

export default class SetValidatorConfig extends SolanaCommand {
  static id = 'store:set_validator_config'
  static category = CONTRACT_LIST.STORE

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

  makeRawTransaction = async (signer: PublicKey) => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const input = this.makeInput(this.flags.input)

    const state = new PublicKey(this.flags.state)
    const threshold = new BN(input.threshold)
    const feed = new PublicKey(input.feed)

    const data = program.coder.instruction.encode('set_validator_config', {
      flaggingThreshold: threshold,
    })

    const accounts: AccountMeta[] = [
      {
        pubkey: state,
        isSigner: false,
        isWritable: true,
      },
      {
        pubkey: signer,
        isSigner: true,
        isWritable: false,
      },
      {
        pubkey: feed,
        isSigner: false,
        isWritable: true,
      },
    ]

    const rawTx: RawTransaction = {
      data,
      accounts,
      programId: storeProgram.programId,
    }

    return [rawTx]
  }

  execute = async () => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
    const tx = makeTx(rawTx)
    logger.debug(tx)
    logger.info(`Setting store config on ${this.flags.state.toString()}...`)
    logger.loading('Sending tx...')
    const txhash = await this.sendTx(tx, [this.wallet.payer], contract.idl)
    logger.success(`Validator config on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.flags.state),
          contract: this.flags.state,
        },
      ],
    } as Result<TransactionResponse>
  }
}
