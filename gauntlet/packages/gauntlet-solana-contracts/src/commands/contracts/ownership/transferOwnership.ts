import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'
import RDD from '../../../lib/rdd'

export const makeTransferOwnershipCommand = (
  contractId: CONTRACT_LIST,
  readOwnership: SolanaConstructor,
): SolanaConstructor => {
  return class TransferOwnership extends SolanaCommand {
    static id = `${contractId}:transfer_ownership`
    static category = contractId
    ReadOwnership = readOwnership

    static examples = [
      `yarn gauntlet ${contractId}:transfer_ownership --network=devnet --to=[PROPOSED_OWNER] [PROGRAM_STATE]`,
    ]

    constructor(flags, args) {
      super(flags, args)

      this.require(!!this.flags.to, 'Please provide flags with "to"')
      this.require(!!this.flags.rdd, 'Please provide RDD path with "rdd" flag')
      this.requireArgs('Please provide the state as an arg!')
    }

    buildCommand = async (flags, args) => {
      const contract = getContract(contractId, '')
      const address = contract.programId.toString()
      this.program = this.loadProgram(contract.idl, address)

      return this
    }

    makeRawTransaction = async (signer: PublicKey) => {
      const state = new PublicKey(this.args[0])
      const proposedOwner = new PublicKey(this.flags.to)

      const tx = this.program.instruction.transferOwnership(proposedOwner, {
        accounts: {
          state: state,
          authority: signer,
        },
      })

      return [tx]
    }

    readOwner = async () => {
      const readOwnershipCommand = new this.ReadOwnership(this.flags, this.args)
      readOwnershipCommand.provider = this.provider
      const { owner } = (await readOwnershipCommand.execute()) as any
      return owner
    }

    beforeExecute = async () => {
      const owner = await this.readOwner()
      const contract = RDD.getContractFromRDD(RDD.load(this.flags.network, this.flags.rdd), this.args[0])

      logger.info(`Transferring Ownership of contract of type "${contract.type}":
      - Contract: ${contract.address} ${contract.description ? '- ' + contract.description : ''}
      - Current Owner: ${owner.toString()}
      - Next Owner: ${this.flags.to}`)
      await prompt('Continue?')
    }

    execute = async () => {
      await this.buildCommand(this.flags, this.args)
      await this.beforeExecute()

      const signer = this.wallet.publicKey
      const rawTx = await this.makeRawTransaction(signer)
      await this.simulateTx(signer, rawTx)
      const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, this.program.idl)(rawTx)

      logger.success(`Ownership transferred to ${new PublicKey(this.flags.to)} on tx ${txhash}`)

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
