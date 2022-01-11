import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Token, TOKEN_PROGRAM_ID, ASSOCIATED_TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  operators: {
    transmitter: string
    payee: string
  }[]
  // Allows to set payees that do not have a token generated address
  allowFundRecipient?: boolean
}

export default class SetPayees extends SolanaCommand {
  static id = 'ocr2:set_payees'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_payees --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const aggregator = rdd.contracts[this.flags.state]
    const aggregatorOperators: string[] = aggregator.oracles.map((o) => o.operator)
    const operators = aggregatorOperators.map((operator) => ({
      transmitter: rdd.operators[operator].ocrNodeAddress[0],
      payee: rdd.operators[operator].adminAddress,
    }))
    return {
      operators,
      allowFundRecipient: false,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
    this.requireFlag('link', 'Provide a valid link address')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const owner = this.wallet.payer
    const input = this.makeInput(this.flags.input)
    const state = new PublicKey(this.flags.state)

    const info = await program.account.state.fetch(state)
    const token = new Token(
      this.provider.connection,
      new PublicKey(this.flags.link),
      TOKEN_PROGRAM_ID,
      this.wallet.payer,
    )

    this.flags.TESTING_ONLY_IGNORE_PAYEE_VALIDATION &&
      logger.warn('TESTING_ONLY_IGNORE_PAYEE_VALIDATION flag is enabled')

    if (!this.flags.TESTING_ONLY_IGNORE_PAYEE_VALIDATION) {
      const areValidPayees = (
        await Promise.all(
          input.operators.map(async ({ payee }) => {
            try {
              const to = await Token.getAssociatedTokenAddress(
                ASSOCIATED_TOKEN_PROGRAM_ID,
                TOKEN_PROGRAM_ID,
                token.publicKey,
                new PublicKey(payee),
              )
              const info = await token.getAccountInfo(to)
              return !!info.address
            } catch (e) {
              logger.error(`Payee with address ${payee} does not have a valid Token recipient address`)
              return false
            }
          }),
        )
      ).every((isValid) => isValid)

      this.require(
        areValidPayees || !!input.allowFundRecipient,
        'Every payee needs to have a valid token recipient address',
      )
    }
    const payeeByTransmitter = input.operators.reduce(
      (agg, operator) => ({
        ...agg,
        [new PublicKey(operator.transmitter).toString()]: new PublicKey(operator.payee),
      }),
      {},
    )

    // Set the payees in the same order the oracles are saved in the contract. The length of the payees need to be same as the oracles saved
    const payees = info.oracles.xs
      .slice(0, info.oracles.len)
      .map(({ transmitter }) => payeeByTransmitter[new PublicKey(transmitter).toString()])

    logger.log('Payees information:', input)
    await prompt('Continue setting payees?')

    const tx = await program.rpc.setPayees(payees, {
      accounts: {
        state: state,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

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
