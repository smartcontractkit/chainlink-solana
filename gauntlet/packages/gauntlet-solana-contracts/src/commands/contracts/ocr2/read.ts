import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class ReadState extends SolanaCommand {
  static id = 'ocr2:read_state'
  static category = CONTRACT_LIST.OCR_2

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state""')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.publicKey.toString())

    const state = new PublicKey(this.flags.state)
    // read could be abstract. account.accessController is just the name of the account that can be got form the camelcase(schema.accounts[x].name)
    const data = await program.account.state.fetch(state)
    const transmissions = new PublicKey(data.transmissions)
    const transmissionsData = await program.account.transmissions.fetch(transmissions)

    const validTransmissions = transmissionsData.transmissions
      .map(({ answer, timestamp }) => {
        const value = new BN(answer)
        if (!!value.toNumber()) {
          return { value: value.toString(), timestamp }
        }
      })
      .filter((v) => !!v)
    console.log('DATA:', data)
    console.log('TRANSMISSIONS:', validTransmissions)
    return {} as Result<TransactionResponse>
  }
}
