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

    makeRawTransaction = async (signer: PublicKey) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const state = new PublicKey(this.flags.state)

      const tx = program.instruction.acceptOwnership({
        accounts: {
          store: state,
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
      await prompt(`Accepting ownership of ${contractId} state (${this.flags.state.toString()}). Continue?`)
      const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx)
      logger.success(`Accepted ownership on tx hash: ${txhash}`)
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
