import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export const makeAcceptOwnershipCommand = (contractId: CONTRACT_LIST): SolanaConstructor => {
  return class AcceptOwnership extends SolanaCommand {
    static id = `${contractId}:accept_ownership`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:accept_ownership --network=devnet --state=[PROGRAM_STATE]`]

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

      await prompt(`Accepting ownership of ${contractId} state (${state.toString()}). Continue?`)

      const tx = await program.rpc.acceptOwnership({
        accounts: {
          // Store contract expects an store account instead of a state acc
          ...(contractId === CONTRACT_LIST.STORE && { store: state }),
          ...(contractId !== CONTRACT_LIST.STORE && { state }),
          authority: owner.publicKey,
        },
        signers: [owner],
      })

      logger.success(`Accepted ownership on tx ${tx}`)

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
