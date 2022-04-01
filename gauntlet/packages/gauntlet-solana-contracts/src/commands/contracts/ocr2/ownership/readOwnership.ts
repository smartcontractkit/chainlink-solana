import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class ReadState extends SolanaCommand {
  static id = 'ocr2:read_ownership'
  static category = CONTRACT_LIST.OCR_2

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    const state = new PublicKey(this.args[0])
    const data = await program.account.state.fetch(state)
    return {
      owner: new PublicKey(data.config.owner),
    } as any
  }
}
