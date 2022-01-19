import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { Idl } from '@project-serum/anchor'
import { AccountMeta, PublicKey, Transaction, TransactionInstruction } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'
import { parseContractErrors, makeTx } from '../../../lib/utils'

export const makeTransferOwnershipCommand = (contractId: CONTRACT_LIST): SolanaConstructor => {
  return class TransferOwnership extends SolanaCommand {
    static id = `${contractId}:transfer_ownership`
    static category = contractId
    idl: Idl

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
      this.idl = contract.idl
      const address = contract.programId.toString()
      const program = this.loadProgram(contract.idl, address)

      const state = new PublicKey(this.flags.state)
      const proposedOwner = new PublicKey(this.flags.to)

      await prompt(
        `Transfering ownership of ${contractId} state (${state.toString()}) to: (${proposedOwner.toString()}). Continue?`,
      )

      const data = program.coder.instruction.encode('transfer_ownership', {
        proposedOwner,
      })

      const accounts: AccountMeta[] = [
        {
          pubkey: state,
          isSigner: false,
          isWritable: true,
        },
        {
          pubkey: signer,
          isSigner: true,
          isWritable: false,
        },
      ]

      const rawTx: RawTransaction = {
        data,
        accounts,
        programId: contract.programId,
      }

      return [rawTx]
    }

    execute = async () => {
      const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
      const tx = await makeTx(rawTx)
      logger.debug(tx)

      logger.loading('Sending tx...')
      const txhash = await parseContractErrors(this.provider.send(tx, [this.wallet.payer]), this.idl)
  
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
