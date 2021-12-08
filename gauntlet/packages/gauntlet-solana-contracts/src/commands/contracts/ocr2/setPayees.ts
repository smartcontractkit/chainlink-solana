import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  operators: {
    transmitter: string
    payee: string
  }[]
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
      transmitter: rdd.operators[operator].nodeAddress[0],
      payee: rdd.operators[operator].payeeAddress,
    }))
    return {
      operators,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const owner = this.wallet.payer
    const input = this.makeInput(this.flags.input)
    const state = new PublicKey(this.flags.state)

    const info = await program.account.state.fetch(state)
    const payeeByTransmitter = input.operators.reduce(
      (agg, operator) => ({
        ...agg,
        [new PublicKey(operator.transmitter).toString()]: operator.payee,
      }),
      {},
    )
    // Set the payees in the same order the oracles are saved in the contract. The length of the payees need to be same as the oracles saved
    const payees = info.oracles
      .map(({ transmitter }) => payeeByTransmitter[new PublicKey(transmitter).toString()])
      .slice(0, info.config.n)

    logger.loading('Setting payees...')
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
