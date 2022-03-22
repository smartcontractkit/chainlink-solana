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

    makeRawTransaction = async (signer) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const state = new PublicKey(this.flags.state)

      const ix = program.instruction.close({
        accounts: {
          state: state,
          receiver: signer,
          authority: signer,
        },
      })

      return [ix]
    }

    execute = async () => {
      const state = new PublicKey(this.flags.state)
      const ixs = await this.makeRawTransaction(this.wallet.publicKey)

      await prompt(`Continue closing ${contractId} state with address ${state.toString()}?`)

      const tx = await this.signAndSendRawTx(ixs)

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
