import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey, Transaction, TransactionInstruction } from '@solana/web3.js'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  transmissions: string
  store: string
}

export default class SetWriter extends SolanaCommand {
  static id = 'store:set_writer'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:set_writer --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --ocrState=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.ocrState, 'Please provide flags with "ocrState"')
  }

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const agg = rdd.contracts[this.flags.ocrState]
    return {
      transmissions: agg.transmissionsAccount,
      store: agg.storeAccount,
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
    // const owner = this.wallet.payer

    const storeState = new PublicKey(input.store || this.flags.state)
    const ocr2State = new PublicKey(this.flags.ocrState)
    const feedState = new PublicKey(input.transmissions)

    const [storeAuthority, _storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('store')), ocr2State.toBuffer()],
      ocr2Program.programId,
    )

    console.log(`Setting store writer on Store (${storeState.toString()}) and Feed (${feedState.toString()})`)

    const data = storeProgram.coder.instruction.encode('set_writer', {
      writer: storeAuthority,
    })

    const accounts: AccountMeta[] = [
      {
        pubkey: storeState,
        isSigner: false,
        isWritable: true,
      },
      {
        pubkey: signer,
        isSigner: true,
        isWritable: false,
      },
      {
        pubkey: feedState,
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
    const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
    const tx = rawTx.reduce(
      (tx, meta) =>
        tx.add(
          new TransactionInstruction({
            programId: meta.programId,
            keys: meta.accounts,
            data: meta.data,
          }),
        ),
      new Transaction(),
    )

    logger.loading('Sending tx...')
    logger.debug(tx)
    const txhash = await this.provider.send(tx, [this.wallet.payer])
    logger.success(`Writer set on tx hash: ${txhash}`)

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
