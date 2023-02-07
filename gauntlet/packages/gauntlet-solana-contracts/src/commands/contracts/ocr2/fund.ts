import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { createTransferInstruction, getAssociatedTokenAddress } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { TOKEN_DECIMALS } from '../../../lib/constants'

type Input = {
  amount: number | string
  link: string
}

export default class Fund extends SolanaCommand {
  static id = 'ocr2:fund'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:fund --network=devnet --amount=[AMOUNT in LINK] [AGGREGATOR_ADDRESS]']

  input: Input

  constructor(flags, args) {
    super(flags, args)

    this.requireArgs('Please provide an aggregator address')
    this.requireFlag('amount', 'Provide an --amount flag')
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)

    return this
  }

  makeInput = (input?: Input) => {
    if (input) return input

    const link = this.flags.link || process.env.LINK
    this.require(link, 'Please provide a link address with --link or env LINK')
    return {
      amount: (BigInt(this.flags.amount) * BigInt(10) ** BigInt(TOKEN_DECIMALS)).toString(),
      link: this.flags.link || process.env.LINK,
    }
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const state = new PublicKey(this.args[0])
    const linkPublicKey = new PublicKey(this.input.link)

    // Resolve the tokenVault from the aggregator state account
    const stateAccount = (await this.program.account.state.fetch(state)) as any
    const tokenVault = stateAccount.config.tokenVault
    const tokenMint = stateAccount.config.tokenMint
    this.require(tokenMint.equals(linkPublicKey), 'LINK does not match aggregator.config.tokenMint')

    const from = await getAssociatedTokenAddress(linkPublicKey, signer)
    const amount = new BN(this.input.amount)
    logger.loading(
      `Transferring ${amount.toString()} tokens to ${state.toString()} aggregator token vault ${tokenVault.toString()}...`,
    )

    const ix = createTransferInstruction(from, tokenVault, signer, amount.toNumber())

    return [
      {
        ...ix,
        // createTransferInstruction does not return the PublicKey type
        keys: ix.keys.map((k) => ({ ...k, pubkey: new PublicKey(k.pubkey) })),
      },
    ]
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)
    // use local wallet as signer
    const signer = this.wallet.publicKey
    const rawTx = await this.makeRawTransaction(signer)
    await prompt('Continue funding feed?')
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Tokens sent on tx hash: ${txhash}`)

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
