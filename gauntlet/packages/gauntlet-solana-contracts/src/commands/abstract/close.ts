import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../..'
import { SolanaConstructor } from '../../lib/types'

export const makeCloseCommand = (contractId: CONTRACT_LIST): SolanaConstructor => {
  return class Close extends SolanaCommand {
    static id = `${contractId}:close`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:close --network=devnet --state=[PROGRAM_STATE]`]

    constructor(flags, args) {
      super(flags, args)

      this.require(!!this.flags.state, 'Please provide flags with "state"')
    }

    execute = async () => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const owner = this.wallet.payer

      const state = new PublicKey(this.flags.state)

      await prompt(`Continue closing ${contractId} state with address ${state.toString()}?`)

      const tx = await await program.rpc.close({
        accounts: {
          state: state,
          receiver: owner.publicKey,
          authority: owner.publicKey,
        },
      })

      logger.success(`Closed state ${state.toString()} on tx ${tx}`)

      return {
        responses: [
          {
            tx: this.wrapResponse(tx, state.toString(), { state: state.toString() }),
            contract: state.toString(),
          },
        ],
      }
    }
  }
}
