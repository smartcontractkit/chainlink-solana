import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { utils } from '@coral-xyz/anchor'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

type Input = {
  recipient: string
}

export default class WithdrawPayment extends SolanaCommand {
  static id = 'ocr2:withdraw_payment'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:withdraw_payment --network=devnet --recipient=YOUR_LINK_ACCOUNT <AGGREGATOR_ADDR>',
    'yarn gauntlet ocr2:withdraw_payment --network=devnet --recipient=FTH1Kqvr5BhiAA786DdQVBQYJ1bs5XhKwTEETKCqYwMh 9hBz81AnfoeGgqVqQHKBiAXGJ2hKAs7A2KYFxn5yGgat',
  ]

  input: Input

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input

    if (!this.flags.recipient) {
      throw Error('Please specify a valid LINK --recipient for withdrawal')
    }

    return {
      recipient: this.flags.recipient,
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)
    return this
  }

  beforeExecute = async () => {
    logger.loading(`Executing ${WithdrawPayment.id} from contract ${this.args[0]}`)
    await prompt(`Continue?`)
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const state = new PublicKey(this.args[0])

    const info = (await this.program.account.state.fetch(state)) as any
    const tokenVault = new PublicKey(info.config.tokenVault)
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      this.program.programId,
    )

    const data = this.program.instruction.withdrawPayment({
      accounts: {
        state,
        authority: signer,
        tokenVault: tokenVault,
        vaultAuthority: vaultAuthority,
        tokenProgram: TOKEN_PROGRAM_ID,
        payee: new PublicKey(this.input.recipient),
      },
    })

    return [data]
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)
    // use local wallet as signer
    const signer = this.wallet.publicKey

    const rawTx = await this.makeRawTransaction(signer)
    await this.simulateTx(signer, rawTx)

    await this.beforeExecute()
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Payment withdrew on tx hash: ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.args[0]),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
