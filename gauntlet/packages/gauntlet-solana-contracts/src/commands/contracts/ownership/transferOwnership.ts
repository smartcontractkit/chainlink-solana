import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export const makeTransferOwnershipCommand = (contractId: CONTRACT_LIST): SolanaConstructor => {
  return class TransferOwnership extends SolanaCommand {
    static id = `${contractId}:transfer_ownership`
    static category = contractId

    static examples = [
      `yarn gauntlet ${contractId}:transfer_ownership --network=devnet --state=[PROGRAM_STATE] --to=[PROPOSED_OWNER]`,
    ]

    constructor(flags, args) {
      super(flags, args)

      this.require(!!this.flags.state, 'Please provide flags with "state"')
      this.require(!!this.flags.to, 'Please provide flags with "to"')
    }

    execute = async () => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const owner = this.wallet.payer

      const state = new PublicKey(this.flags.state)
      const proposedOwner = new PublicKey(this.flags.to)

      await prompt(
        `Transfering ownership of ${contractId} state (${state.toString()}) to: (${proposedOwner.toString()}). Continue?`,
      )

      const tx = await program.rpc.transferOwnership(proposedOwner, {
        accounts: {
          // Store contract expects an store account instead of a state acc
          ...(contractId === CONTRACT_LIST.STORE && { store: state }),
          ...(contractId !== CONTRACT_LIST.STORE && { state }),
          authority: owner.publicKey,
        },
        signers: [owner],
      })

      logger.success(`Ownership transferred to ${proposedOwner.toString()} on tx ${tx}`)

      return {
        responses: [
          {
            tx: this.wrapResponse(tx, state.toString(), { state: state.toString() }),
            contract: state.toString(),
          },
        ],
      } as Result<TransactionResponse>
    }
  }
}
