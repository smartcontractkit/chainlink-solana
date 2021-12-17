import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { inspect, Inspection } from '../../../../core/inspection'

type Input = {}

export default class OCR2InspectResponses extends SolanaCommand {
  static id = 'ocr2:inspect:responses'
  static category = CONTRACT_LIST.OCR_2

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input

    return {}
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state""')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.flags.state)
    const input = this.makeInput(this.flags.input)
    const data = await program.account.state.fetch(state)

    const inspections: Inspection[] = []

    const successfulInspection = inspect(inspections)

    return {
      responses: [
        {
          tx: this.wrapInspectResponse(successfulInspection, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
