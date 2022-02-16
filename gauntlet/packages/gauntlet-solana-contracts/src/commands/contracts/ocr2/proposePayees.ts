// TODO: REMOVE

import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'

type Input = {
  operators: {
    transmitter: string
    payee: string
  }[]
  // Allows to set payees that do not have a token generated address
  allowFundRecipient?: boolean
}

export default class ProposePayees extends SolanaCommand {
  static id = 'ocr2:propose_payees'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:propose_payees --network=devnet --rdd=[PATH_TO_RDD] --proposalId=<PROPOSAL_ID> EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:propose_payees --proposalId=<PROPOSAL_ID> EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''
    const rdd = RDD.load(network, rddPath)
    const aggregator = RDD.loadAggregator(network, rddPath, this.args[0])
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

    this.requireFlag('proposalId', 'Provide a valid proposal address')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const input = this.makeInput(this.flags.input)
    const proposal = new PublicKey(this.flags.proposalId)
    const link = new PublicKey(this.flags.link || process.env.LINK)

    const token = new Token(this.provider.connection, link, TOKEN_PROGRAM_ID, {
      publicKey: signer,
      secretKey: Buffer.from([]),
    })

    const areValidPayees = (
      await Promise.all(
        input.operators.map(async ({ payee }) => {
          try {
            const info = await token.getAccountInfo(new PublicKey(payee))
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

    const proposalInfo = await program.account.proposal.fetch(proposal)
    const payeeByTransmitter = input.operators.reduce(
      (agg, operator) => ({
        ...agg,
        [new PublicKey(operator.transmitter).toString()]: new PublicKey(operator.payee),
      }),
      {},
    )

    // Set the payees in the same order the oracles are saved in the proposal. The length of the payees need to be same as the oracles saved
    const payees = proposalInfo.oracles.xs
      .slice(0, proposalInfo.oracles.len)
      .map(({ transmitter }) => payeeByTransmitter[new PublicKey(transmitter).toString()])

    const ix = await program.instruction.proposePayees(token.publicKey, payees, {
      accounts: {
        proposal,
        authority: signer,
      },
    })
    logger.log('Payees information:', input)
    logger.log('Setting the following:', payees)

    const rawTx: RawTransaction = {
      data: ix.data,
      accounts: ix.keys,
      programId: ix.programId,
    }

    return [rawTx]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt('Continue setting payees proposal?')
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Payees proposal set on tx hash: ${txhash}`)

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
