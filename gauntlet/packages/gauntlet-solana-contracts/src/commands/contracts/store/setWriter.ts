import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { makeTx } from '../../../lib/utils'
import RDD from '../../../lib/rdd'

type Input = {
  transmissions: string
  store: string
}

export default class SetWriter extends SolanaCommand {
  static id = 'store:set_writer'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_writer --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC [AGGREGATOR_ADDRESS]',
  ]

  constructor(flags, args) {
    super(flags, args)
  }

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''
    const aggregator = RDD.loadAggregator(network, rddPath, this.args[0])

    return {
      transmissions: aggregator.transmissionsAccount,
      store: aggregator.storeAccount,
    }
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const storeAddress = store.programId.toString()
    const ocr2Address = ocr2.programId.toString()
    const storeProgram = this.loadProgram(store.idl, storeAddress)
    const ocr2Program = this.loadProgram(ocr2.idl, ocr2Address)

    const input = this.makeInput(this.flags.input)

    const storeState = new PublicKey(input.store || this.flags.state)
    const ocr2State = new PublicKey(this.args[0])
    const feedState = new PublicKey(input.transmissions)

    logger.info(
      `Generating data for setting store writer on Store (${storeState.toString()}) and Feed (${feedState.toString()})`,
    )

    const [storeAuthority, _storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('store')), ocr2State.toBuffer()],
      ocr2Program.programId,
    )

    // Resolve the current store owner
    let feedAccount = await storeProgram.account.transmissions.fetch(feedState)

    const tx = storeProgram.instruction.setWriter(storeAuthority, {
      accounts: {
        feed: feedState,
        owner: feedAccount.owner,
        authority: signer,
      },
    })
    return [tx]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    const txhash = await this.signAndSendRawTx(rawTx)
    const input = this.makeInput(this.flags.input)
    const state = input.store || this.flags.state
    logger.success(`Writer set on tx hash: ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, state),
          contract: state,
        },
      ],
    } as Result<TransactionResponse>
  }
}
