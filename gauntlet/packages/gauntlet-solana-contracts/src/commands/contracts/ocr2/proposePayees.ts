import { Result } from '@chainlink/gauntlet-core'
import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, logger } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'
import { printDiff } from '../../../lib/diff'

type Input = {
  operators: {
    transmitter: string
    payee: string
  }[]
  proposalId: string
  // Allows to set payees that do not have a token generated address
  allowFundRecipient?: boolean
}

type ContractInput = {
  payeeByTransmitter: {
    [key: string]: PublicKey
  }
}

export default class ProposePayees extends SolanaCommand {
  static id = 'ocr2:propose_payees'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:propose_payees --network=<NETWORK> --rdd=<PATH_TO_RDD> --proposalId=<PROPOSAL_ID> <AGGREGATOR_ADDRESS>',
    'yarn gauntlet ocr2:propose_payees --proposalId=<PROPOSAL_ID> EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  input: Input
  contractInput: ContractInput

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input

    const rdd = RDD.load(this.flags.network, this.flags.rdd)

    const aggregator = rdd.contracts[this.args[0]]
    const aggregatorOperators: string[] = aggregator.oracles.map((o) => o.operator)
    const operators = aggregatorOperators.map((operator) => ({
      transmitter: rdd.operators[operator].ocrNodeAddress[0],
      payee: rdd.operators[operator].adminAddress,
    }))

    return {
      operators,
      allowFundRecipient: false,
      proposalId: this.flags.proposalId || this.flags.configProposal,
    }
  }

  makeContractInput = (input: Input): ContractInput => {
    const payeeByTransmitter = input.operators.reduce(
      (agg, operator) => ({
        ...agg,
        [new PublicKey(operator.transmitter).toString()]: new PublicKey(operator.payee),
      }),
      {},
    )

    return {
      payeeByTransmitter,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(
      !!this.flags.proposalId || !!this.flags.configProposal,
      'Please provide Config Proposal ID with flag "proposalId" or "configProposal"',
    )
    this.requireArgs('Please provide an aggregator address as arg')
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = await this.makeInput(flags.input)
    this.contractInput = await this.makeContractInput(this.input)

    return this
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const link = new PublicKey(this.flags.link || process.env.LINK)

    const token = new Token(this.provider.connection, link, TOKEN_PROGRAM_ID, {
      publicKey: signer,
      secretKey: Buffer.from([]),
    })

    const areValidPayees = (
      await Promise.all(
        Object.entries(this.contractInput.payeeByTransmitter).map(async ([transmitter, payee]) => {
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
      areValidPayees || !!this.input.allowFundRecipient,
      'Every payee needs to have a valid token recipient address',
    )

    // Set the payees in the same order the oracles are saved in the proposal
    // The length of the payees need to be same as the oracles saved
    const proposal = new PublicKey(this.input.proposalId)
    const proposalInfo = await this.program.account.proposal.fetch(proposal)
    const payees = proposalInfo.oracles.xs
      .slice(0, proposalInfo.oracles.len)
      .map(({ transmitter }) => this.contractInput.payeeByTransmitter[transmitter.toString()])

    const ix = this.program.instruction.proposePayees(token.publicKey, payees, {
      accounts: {
        proposal,
        authority: signer,
      },
    })
    return [ix]
  }

  beforeExecute = async () => {
    const state = new PublicKey(this.args[0])
    const proposal = new PublicKey(this.input.proposalId)
    const contractState = await this.program.account.state.fetch(state)
    const proposalState = await this.program.account.proposal.fetch(proposal)

    const payeesInContract = {
      oracles: contractState.oracles.xs
        .slice(0, contractState.oracles.len.toNumber())
        .map(({ transmitter, payee }) => ({
          transmitter: transmitter.toString(),
          payee: payee.toString(),
        })),
    }

    const proposedPayees = {
      oracles: proposalState.oracles.xs.slice(0, proposalState.oracles.len).map(({ transmitter }) => ({
        transmitter: transmitter.toString(),
        payee: this.contractInput.payeeByTransmitter[transmitter.toString()].toString(),
      })),
    }

    logger.info(`Proposed payees for contract ${this.args[0]}`)
    printDiff(payeesInContract, proposedPayees)

    await prompt('Continue?')
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)

    const signer = this.wallet.publicKey

    const rawTx = await this.makeRawTransaction(signer)
    await this.simulateTx(signer, rawTx)
    await this.beforeExecute()

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
