import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  transmissions: string
}

export default class SetWriter extends SolanaCommand {
  static id = 'store:set_writer'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_writer --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --ocrState=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
    this.require(!!this.flags.ocrState, 'Please provide flags with "ocrState"')
  }

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const agg = rdd[this.flags.ocrState]
    return {
      transmissions: agg.transmissionsAccount,
    }
  }

  execute = async () => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = store.programId.toString()
    const storeProgram = this.loadProgram(store.idl, address)
    const ocr2Program = this.loadProgram(ocr2.idl, address)

    const input = this.makeInput(this.flags.input)
    const owner = this.wallet.payer

    const storeState = new PublicKey(this.flags.state)
    const ocr2State = new PublicKey(this.flags.ocrState)
    const feedState = new PublicKey(input.transmissions)

    const [storeAuthority, _storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('store')), ocr2State.toBuffer()],
      ocr2Program.programId,
    )

    console.log(`Setting store writer on ${storeState.toString()} and ${feedState.toString()}`)

    const tx = await storeProgram.rpc.setWriter(storeAuthority, {
      accounts: {
        store: storeState,
        feed: feedState,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

    logger.success(`Set writer on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, storeState.toString(), { state: storeState.toString() }),
          contract: storeState.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
