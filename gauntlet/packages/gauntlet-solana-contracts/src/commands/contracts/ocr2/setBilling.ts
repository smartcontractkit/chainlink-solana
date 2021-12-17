import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { parseIdlErrors, ProgramError } from '@project-serum/anchor'
import { AccountMeta, PublicKey, Transaction, TransactionInstruction } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  observationPaymentGjuels: number | string
  transmissionPaymentGjuels: number | string
}

type SolanaRawTransaction = {
  data: Buffer
  accounts: AccountMeta[]
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
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)

    const input = this.makeInput(
      ({
        observationPaymentGjuels: '10',
        transmissionPaymentGjuels: '20',
      } as Input) || this.flags.input,
    )

    const info = await program.account.state.fetch(state)
    const billingAC = new PublicKey(info.config.billingAccessController)
    logger.loading('Generating billing tx information...')

    const data = program.coder.instruction.encode('set_billing', {
      observationPaymentGjuels: new BN(input.observationPaymentGjuels),
      transmissionPaymentGjuels: new BN(input.transmissionPaymentGjuels),
    })

    const accounts: AccountMeta[] = [
      {
        pubkey: state,
        isSigner: false,
        isWritable: true,
      },
      {
        pubkey: this.wallet.payer.publicKey,
        isSigner: true,
        isWritable: false,
      },
      {
        pubkey: billingAC,
        isSigner: false,
        isWritable: false,
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
    if (this.flags.multisig != undefined) {
      console.info("Write below tx data to JSON file, to be parsed by multisig:tx command")
      console.info(JSON.stringify(rawTx))
      return {
        responses: [
          {
            tx: this.wrapResponse('multisig', this.flags.state),
            contract: this.flags.state,
          },
        ],
      } as Result<TransactionResponse>
    }

    const tx = new Transaction()
    tx.add(
      new TransactionInstruction({
        programId: rawTx.programId,
        keys: rawTx.accounts,
        data: rawTx.data,
      }),
    )

    logger.loading('Sending tx...')
    const txhash = await this.provider.send(tx, [this.wallet.payer])

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
