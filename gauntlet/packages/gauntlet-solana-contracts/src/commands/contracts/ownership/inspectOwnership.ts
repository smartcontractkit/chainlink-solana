import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Idl, Program } from '@project-serum/anchor'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

type Ownership = {
  owner: string
  proposedOwner?: string
}

export type GetOwnership = (program: Program<Idl>, account: PublicKey) => Promise<Ownership>

export const makeInspectOwnershipCommand = (
  contractId: CONTRACT_LIST,
  getOwnership: GetOwnership,
): SolanaConstructor => {
  return class InspectOwnership extends SolanaCommand {
    static id = `${contractId}:inspect_ownership`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:inspect_ownership --network=devnet [CONTRACT_ADDRESS]`]

    constructor(flags, args) {
      super(flags, args)
    }

    execute = async () => {
      // Get contract ownership
      logger.info(`Checking owner of ${this.args[0]}`)
      const contract = getContract(contractId, '')
      const program = this.loadProgram(contract.idl, contract.programId.toString())
      const state = new PublicKey(this.args[0])
      const ownership = await getOwnership(program, state)
      // Log owner of contract
      logger.info(`Owner: ${ownership.owner}`)
      // Log proposed owner of contract if it exists
      if (!!ownership.proposedOwner) {
        logger.info(`Proposed Owner: ${ownership.proposedOwner}`)
      }
      // Return response
      return {
        data: {
          owner: ownership.owner,
        },
      } as Result<TransactionResponse>
    }
  }
}
