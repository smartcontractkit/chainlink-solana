import { Result } from '@chainlink/gauntlet-core'
import { assertions, logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export const makeInspectOwnershipCommand = (
  contractId: CONTRACT_LIST,
  getOwner: (program, state) => Promise<PublicKey>,
): SolanaConstructor => {
  return class InspectOwnership extends SolanaCommand {
    static id = `${contractId}:inspect_ownership`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:inspect_ownership --network=devnet [CONTRACT_ADDRESS]`]

    constructor(flags, args) {
      super(flags, args)
    }

    execute = async () => {
      // Get contract owner
      logger.info(`Checking owner of ${this.args[0]}`)
      const contract = getContract(contractId, '')
      const program = this.loadProgram(contract.idl, contract.programId.toString())
      const state = new PublicKey(this.args[0])
      const owner = await getOwner(program, state)
      // Log owner of contract
      logger.info(`Owner: ${owner}`)
      // Return response
      return {
        data: {
          owner,
        },
      } as Result<TransactionResponse>
    }
  }
}
