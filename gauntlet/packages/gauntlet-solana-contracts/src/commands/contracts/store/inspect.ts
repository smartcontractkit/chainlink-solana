import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/solana-gauntlet'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { inspection } from '@chainlink/gauntlet-core/dist/utils'

type Input = {}

export default class Inspect extends SolanaCommand {
  static id = 'store:inspect'
  static category = CONTRACT_LIST.OCR_2

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input

    return {}
  }

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const program = this.loadProgram(store.idl, store.programId.toString())

    const state = new PublicKey(this.args[0])
    const data = await program.account.store.fetch(state)

    console.log('DATA:', data)

    const inspections: inspection.Inspection[] = []

    const successfulInspection = inspection.inspect(inspections)

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
