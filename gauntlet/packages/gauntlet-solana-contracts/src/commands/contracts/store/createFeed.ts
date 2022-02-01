import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { RawTransaction, SolanaCommand } from '@chainlink/gauntlet-solana'
import { AccountMeta, Keypair, PublicKey, SystemProgram } from '@solana/web3.js'
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

  makeRawTransaction = async (signer: PublicKey, feed?: PublicKey): Promise<RawTransaction[]> => {
    if (!feed) throw new Error('Feed account is required')
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const input = this.makeInput(this.flags.input)

    const store = new PublicKey(input.store)

    const granularity = new BN(input.granularity)
    const liveLength = new BN(input.liveLength)
    const length = new BN(this.flags.length || 8096)
    const feedAccountLength = new BN(8 + 128 + length.toNumber() * 24)
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
    `)

    const data = program.coder.instruction.encode('create_feed', {
      description,
      decimals,
      granularity,
      liveLength,
    })

    const accounts: AccountMeta[] = [
      {
        pubkey: store,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: feed,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: signer,
        isWritable: false,
        isSigner: true,
      },
    ]

    const transmissionsCreationInstruction = await SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: feed,
      space: feedAccountLength.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(feedAccountLength.toNumber()),
      programId: program.programId,
    })

    return [
      {
        data: transmissionsCreationInstruction.data,
        accounts: transmissionsCreationInstruction.keys,
        programId: transmissionsCreationInstruction.programId,
      },
      {
        data,
        accounts,
        programId: program.programId,
      },
    ]
  }

  execute = async () => {
    const storeProgram = getContract(CONTRACT_LIST.STORE, '')
    const address = storeProgram.programId.toString()
    const program = this.loadProgram(storeProgram.idl, address)

    const feed = Keypair.generate()

    const rawTxs = await this.makeRawTransaction(this.wallet.publicKey, feed.publicKey)
    await prompt('Continue creating new Transmissions Feed?')

    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTxs, [feed])
    logger.success(`Transmissions feed created on tx ${txhash}`)

    return {
      data: {
        transmissions: feed.publicKey.toString(),
      },
      responses: [
        {
          tx: this.wrapResponse(txhash, feed.toString(), {
            state: feed.toString(),
            transmissions: feed.toString(),
          }),
          contract: feed.toString(),
        },
      ],
    }
  }
}
