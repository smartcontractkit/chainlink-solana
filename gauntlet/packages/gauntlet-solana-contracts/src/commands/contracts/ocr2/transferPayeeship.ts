import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

type Input = {
  transmitter: string
  proposedPayee: string
}

type ContractInput = {
  transmitter: string
  payee: PublicKey
  proposedPayee: PublicKey
}

export default class TransferPayeeship extends SolanaCommand {
  static id = 'ocr2:transfer_payeeship'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:transfer_payeeship --network=<NETWORK> --transmitter=<TRANSMITTER> --proposedPayee=<PROPOSED_PAYEE> <CONTRACT>',
  ]

  input: Input
  contractInput: ContractInput

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input

    if (!this.flags.transmitter) {
      throw Error('Please specify a valid transmitter (--transmitter))')
    }

    if (!this.flags.proposedPayee) {
      throw Error('Please specify a valid proposed payee (--proposedPayee)')
    }

    return {
      transmitter: this.flags.transmitter,
      proposedPayee: this.flags.proposedPayee,
    }
  }

  makeContractInput = async (input: Input): Promise<ContractInput> => {
    const state = new PublicKey(this.args[0])
    const contractState = (await this.program.account.state.fetch(state)) as any
    const contractOracles = contractState.oracles?.xs.slice(0, contractState.oracles.len.toNumber())
    const oracle = contractOracles.find(({ transmitter }) => transmitter.toString() == input.transmitter)

    if (!oracle) {
      throw Error(`No oracle found with the transmitter id ${input.transmitter}`)
    }

    return {
      transmitter: input.transmitter,
      payee: new PublicKey(oracle.payee),
      proposedPayee: new PublicKey(input.proposedPayee),
    }
  }

  constructor(flags, args) {
    super(flags, args)
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)
    this.contractInput = await this.makeContractInput(this.input)

    return this
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const data = this.program.instruction.transferPayeeship({
      accounts: {
        state: new PublicKey(this.args[0]),
        authority: signer,
        transmitter: this.contractInput.transmitter,
        payee: this.contractInput.payee,
        proposedPayee: this.contractInput.proposedPayee,
      },
    })

    return [data]
  }

  beforeExecute = async () => {
    logger.loading(
      `Transferring payeeship for transmitter ${this.contractInput.transmitter}: ${this.contractInput.payee} -> ${this.contractInput.proposedPayee}`,
    )
    await prompt(`Continue?`)
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)

    const signer = this.wallet.publicKey

    const rawTx = await this.makeRawTransaction(signer)
    await this.simulateTx(signer, rawTx)
    await this.beforeExecute()

    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Payeeship transferred on tx hash: ${txhash}`)

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
