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
      `yarn gauntlet ${contractId}:transfer_ownership --network=devnet --to=[PROPOSED_OWNER] [PROGRAM_STATE]`,
    ]

    constructor(flags, args) {
      super(flags, args)

      this.require(!!this.flags.to, 'Please provide flags with "to"')
      this.requireArgs('Please provide the state as an arg!')
    }

    makeRawTransaction = async (signer: PublicKey) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const state = new PublicKey(this.args[0])
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
      const state = this.args[0]

      const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
      await prompt(`Transferring ownership of ${contractId} state (${state}). Continue?`)
      const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx)
      logger.success(`Ownership transferred to ${new PublicKey(this.flags.to)} on tx ${txhash}`)
      return {
        responses: [
          {
            tx: this.wrapResponse(txhash, state),
            contract: state,
          },
        ],
      } as Result<TransactionResponse>
    }
  }
}
