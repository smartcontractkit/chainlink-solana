import { Result } from '@chainlink/gauntlet-core'
import { assertions, logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export const makeInspectOwnershipCommand = (contractId: CONTRACT_LIST): SolanaConstructor => {
  return class InspectOwnership extends SolanaCommand {
    static id = `${contractId}:inspect_ownership`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:inspect_ownership --network=devnet [CONTRACT_ADDRESS]`]

    constructor(flags, args) {
      super(flags, args)
    }

    execute = async () => {
      // Assert that only one argument was inputted
      assertions.assert(this.args.length == 1, `Expected 1 argument, got ${this.args.length}`)
      // Get contract state
      logger.info('Checking owner of ' + this.args[0])
      const contract = getContract(contractId, '')
      const program = this.loadProgram(contract.idl, contract.programId.toString())
      const state = new PublicKey(this.args[0])
      const onChainState = await program.account.state.fetch(state)
      // Log owner of contract
      const onChainOwner = onChainState.config.owner
      logger.info('Owner is ' + onChainOwner)
      // Return responses
      return {
        responses: [onChainOwner],
      } as Result<TransactionResponse>
    }
  }
}
