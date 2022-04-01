import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class ReadState extends SolanaCommand {
  static id = 'access_controller:read_ownership'
  static category = CONTRACT_LIST.ACCESS_CONTROLLER

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const accessController = getContract(CONTRACT_LIST.ACCESS_CONTROLLER, '')
    const program = this.loadProgram(accessController.idl, accessController.programId.toString())

    const state = new PublicKey(this.args[0])
    const data = await program.account.accessController.fetch(state)
    return {
      owner: data.owner,
    } as any
  }
}
