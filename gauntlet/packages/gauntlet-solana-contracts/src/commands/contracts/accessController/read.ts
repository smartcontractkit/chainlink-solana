import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class ReadState extends SolanaCommand {
  static id = 'access_controller:read_state'
  static category = CONTRACT_LIST.ACCESS_CONTROLLER

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state""')
  }

  execute = async () => {
    const accessController = getContract(CONTRACT_LIST.ACCESS_CONTROLLER, '')
    const program = this.loadProgram(accessController.idl, accessController.programId.toString())

    const state = new PublicKey(this.flags.state)
    // read could be abstract. account.accessController is just the name of the account that can be got form the camelcase(schema.accounts[x].name)
    const data = await program.account.accessController.fetch(state)

    console.log(data)
    return {} as Result<TransactionResponse>
  }
}
