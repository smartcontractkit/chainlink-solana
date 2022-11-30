import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'

type Input = {
  transmissions: string
}

export default class SetWriter extends SolanaCommand {
  static id = 'store:set_writer'
  static category = CONTRACT_LIST.STORE

  static examples = ['yarn gauntlet store:set_writer --network=devnet [AGGREGATOR_ADDRESS]']

  constructor(flags, args) {
    super(flags, args)
  }

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const aggregator = RDD.loadAggregator(this.args[0], this.flags.network, this.flags.rdd)

    return {
      transmissions: aggregator.transmissionsAccount,
    }
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = store.programId.toString()
    const program = this.loadProgram(store.idl, address)

    const input = this.makeInput(this.flags.input)

    const ocr2State = new PublicKey(this.args[0])
    const feedState = new PublicKey(input.transmissions)

    logger.info(
      `Generating data for setting feed writer on feed (${feedState.toString()}) and Feed (${feedState.toString()})`,
    )

    const [storeAuthority, _storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('store')), ocr2State.toBuffer()],
      ocr2.programId,
    )

    // Resolve the current store owner
    let feedAccount = (await program.account.transmissions.fetch(feedState)) as any

    const tx = await program.methods
      .setWriter(storeAuthority)
      .accounts({
        feed: feedState,
        owner: feedAccount.owner,
        authority: signer,
      })
      .instruction()
    return [tx]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    const txhash = await this.signAndSendRawTx(rawTx)
    const input = this.makeInput(this.flags.input)
    logger.success(`Writer set on tx hash: ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, input.transmissions),
          contract: input.transmissions,
        },
      ],
    } as Result<TransactionResponse>
  }
}
