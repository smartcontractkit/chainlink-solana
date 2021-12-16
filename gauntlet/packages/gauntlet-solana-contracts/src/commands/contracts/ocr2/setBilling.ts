import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  observationPaymentGjuels: number | string
  transmissionPaymentGjuels: number | string
}

type TxAccount = {
  pubkey: PublicKey
  is_signer: boolean
  is_writable: boolean
}

type SolanaRawTransaction = {
  data: Buffer
  accounts: TxAccount[]
  programId: PublicKey
}

export default class SetBilling extends SolanaCommand {
  static id = 'ocr2:set_billing'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_billing --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const billingInfo = rdd.contracts[this.flags.state]?.billing
    this.require(!!billingInfo?.observationPaymentGjuels, 'Billing information not found')
    this.require(!!billingInfo?.transmissionPaymentGjuels, 'Billing information not found')
    return {
      observationPaymentGjuels: billingInfo.observationPaymentGjuels,
      transmissionPaymentGjuels: billingInfo.transmissionPaymentGjuels,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
  }

  makeRawTransaction = async (): Promise<SolanaRawTransaction> => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const input = this.makeInput(this.flags.input)

    const info = await program.account.state.fetch(state)
    const billingAC = new PublicKey(info.config.billingAccessController)

    logger.loading('Setting billing...')

    // DIRECT EXECUTION
    // const tx = await program.rpc.setBilling(
    //   new BN(input.observationPaymentGjuels),
    //   new BN(input.transmissionPaymentGjuels),
    //   {
    //     accounts: {
    //       state: state,
    //       authority: this.wallet.payer.publicKey,
    //       accessController: billingAC,
    //     },
    //     signers: [this.wallet.payer],
    //   },
    // )
    // */

    const data = program.coder.instruction.encode('set_billing', {
      observation_payment_gjuels: new BN(input.observationPaymentGjuels),
      transmission_payment_gjuels: new BN(input.transmissionPaymentGjuels),
    })

    const accounts: TxAccount[] = [
      {
        pubkey: state,
        is_signer: false,
        is_writable: true,
      },
      {
        pubkey: this.wallet.payer.publicKey,
        is_signer: true,
        is_writable: false,
      },
      {
        pubkey: billingAC,
        is_signer: false,
        is_writable: false,
      },
    ]

    return {
      data,
      accounts,
      programId: ocr2.programId.publicKey,
    }
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction()
    // How can we send this rawTx
    // multisig.execute(rawTx)
    this.wallet.execute(rawTx)
    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
