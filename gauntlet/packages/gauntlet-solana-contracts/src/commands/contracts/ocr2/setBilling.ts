import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { makeTx } from '../../../lib/utils'
import RDD from '../../../lib/rdd'

type Input = {
  observationPaymentGjuels: number | string
  transmissionPaymentGjuels: number | string
}
export default class SetBilling extends SolanaCommand {
  static id = 'ocr2:set_billing'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_billing --network=devnet --rdd=[PATH_TO_RDD] EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:set_billing EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''
    const rdd = RDD.load(network, rddPath)
    const billingInfo = rdd.contracts[this.args[0]]?.billing

    this.require(!!billingInfo?.observationPaymentGjuels, 'Billing information not found')
    this.require(!!billingInfo?.transmissionPaymentGjuels, 'Billing information not found')
    return {
      observationPaymentGjuels: billingInfo.observationPaymentGjuels,
      transmissionPaymentGjuels: billingInfo.transmissionPaymentGjuels,
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)
    const state = new PublicKey(this.args[0])

    const input = this.makeInput(this.flags.input)

    const info = await program.account.state.fetch(state)
    const billingAC = new PublicKey(info.config.billingAccessController)
    logger.loading('Generating billing tx information...')
    logger.log('Billing information:', input)
    const data = program.instruction.setBilling(
      new BN(input.observationPaymentGjuels),
      new BN(input.transmissionPaymentGjuels),
      {
        accounts: {
          state,
          authority: signer,
          accessController: billingAC,
        },
      },
    )

    return [data]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt('Continue setting billing?')
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Billing set on tx hash: ${txhash}`)

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
