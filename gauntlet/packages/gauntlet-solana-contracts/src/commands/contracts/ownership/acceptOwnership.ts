import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { GetOwnership } from './inspectOwnership'
import { SolanaConstructor } from '../../../lib/types'
import RDD from '../../../lib/rdd'

export const makeAcceptOwnershipCommand = (
  contractId: CONTRACT_LIST,
  getOwnership: GetOwnership,
): SolanaConstructor => {
  return class AcceptOwnership extends SolanaCommand {
    static id = `${contractId}:accept_ownership`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:accept_ownership --network=devnet [PROGRAM_STATE]`]

    constructor(flags, args) {
      super(flags, args)

      this.requireArgs('Please provide the state as an arg!')
    }

    buildCommand = async (flags, args) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      this.program = this.loadProgram(contract.idl, address)

      return this
    }

    makeRawTransaction = async (signer: PublicKey) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const state = new PublicKey(this.args[0])

      const tx = await program.methods
        .acceptOwnership()
        .accounts({
          state: state,
          authority: signer,
        })
        .instruction()

      return [tx]
    }

    beforeExecute = async () => {
      const ownership = await getOwnership(this.program, new PublicKey(this.args[0]))
      const contract = RDD.getContractFromRDD(RDD.load(this.flags.network, this.flags.rdd), this.args[0])

      logger.info(`Accepting Ownership of contract of type "${contract.type}":
      - Contract: ${contract.address} ${contract.description ? '- ' + contract.description : ''}
      - Current Owner: ${ownership.owner.toString()}
      - Next Owner (proposed): ${ownership.proposedOwner?.toString()}`)
      await prompt('Continue?')
    }

    execute = async () => {
      await this.buildCommand(this.flags, this.args)
      await this.beforeExecute()

      const signer = this.wallet.publicKey
      const rawTx = await this.makeRawTransaction(signer)
      const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, this.program.idl)(rawTx)

      logger.success(`Accepted ownership on tx hash: ${txhash}`)

      const state = this.args[0]
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
