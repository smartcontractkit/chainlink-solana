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

    makeRawTransaction = async (signer: PublicKey) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const state = new PublicKey(this.flags.state)
      const proposedOwner = new PublicKey(this.flags.to)

      const tx = program.instruction.transferOwnership(proposedOwner, {
        accounts: {
          state: state,
          authority: signer,
        },
      })

      return [tx]
    }

    execute = async () => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
      await prompt(`Transferring ownership of ${contractId} state (${this.flags.state.toString()}). Continue?`)
      const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx)
      logger.success(`Ownership transferred to ${new PublicKey(this.flags.to)} on tx ${txhash}`)
      return {
        responses: [
          {
            tx: this.wrapResponse(txhash, this.flags.state),
            contract: this.flags.state,
          },
        ],
      } as Result<TransactionResponse>
    }
  }
}
