import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { utils } from '@project-serum/anchor'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'

type Input = {
  observationPaymentGjuels: number | string
  transmissionPaymentGjuels: number | string
}
export default class SetBilling extends SolanaCommand {
  static id = 'ocr2:set_billing'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_billing --network=devnet --rdd=[PATH_TO_RDD] <AGGREGATOR_ADDRESS>',
    'yarn gauntlet ocr2:set_billing EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const rdd = RDD.load(this.flags.network, this.flags.rdd)
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
    const tokenVault = new PublicKey(info.config.tokenVault)
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )
    const payees = info.oracles.xs
      .slice(0, info.oracles.len)
      .map((oracle) => ({ pubkey: oracle.payee, isWritable: true, isSigner: false }))

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
          tokenVault: tokenVault,
          vaultAuthority: vaultAuthority,
          tokenProgram: TOKEN_PROGRAM_ID,
        },
        remainingAccounts: payees,
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
