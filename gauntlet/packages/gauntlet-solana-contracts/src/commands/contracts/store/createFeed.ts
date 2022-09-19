import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey, TransactionInstruction, SystemProgram } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'

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

  static examples = ['yarn gauntlet store:create_feed --network=devnet --rdd=[PATH_TO_RDD] [AGGREGATOR_ADDRESS]']

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const aggregator = RDD.loadAggregator(this.args[0], this.flags.network, this.flags.rdd)

    return {
      store: aggregator.storeAccount,
      granularity: aggregator.granularity,
      liveLength: aggregator.liveLength,
      decimals: aggregator.decimals,
      description: aggregator.name,
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  makeRawTransaction = async (signer: PublicKey, feed?: PublicKey): Promise<TransactionInstruction[]> => {
    if (!feed) throw new Error('Feed account is required')
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const input = this.makeInput(this.flags.input)

    const granularity = new BN(input.granularity)
    const liveLength = new BN(input.liveLength)
    const length = new BN(this.flags.length || 140000) // maximum f = 5 (16 oracles)
    const feedAccountLength = new BN(8 + 192 + length.toNumber() * 48) // account discriminator + max transmission header length + (number of transmissions store * size of transmission)
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
      - Historical Length: ${feedAccountLength.toNumber() - liveLength.toNumber()}
      - Total Length: ${feedAccountLength.toNumber()}
      - Feed Account: ${feed.toString()}
    `)

    const transmissionsCreationInstruction = SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: feed,
      space: feedAccountLength.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(feedAccountLength.toNumber()),
      programId: program.programId,
    })

    const ix = await program.methods
      .createFeed(description, decimals, granularity, liveLength)
      .accounts({
        feed,
        authority: signer,
      })
      .instruction()

    return [transmissionsCreationInstruction, ix]
  }

  execute = async () => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const feed = Keypair.generate()

    const rawTxs = await this.makeRawTransaction(this.wallet.publicKey, feed.publicKey)
    await prompt('Continue creating new Transmissions Feed?')

    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTxs, [feed])
    logger.success(`Transmissions feed created at ${feed.publicKey}`)
    logger.success(`TX ${txhash}`)

    return {
      data: {
        transmissions: feed.publicKey.toString(),
      },
      responses: [
        {
          tx: this.wrapResponse(txhash, feed.publicKey.toString(), {
            state: feed.publicKey.toString(),
            transmissions: feed.publicKey.toString(),
          }),
          contract: this.args[0], // continue with undeployed ID
        },
      ],
    }
  }
}
