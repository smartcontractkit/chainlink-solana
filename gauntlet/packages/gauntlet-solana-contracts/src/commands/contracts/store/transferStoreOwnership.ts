import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/solana-gauntlet'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export default class TransferStoreOwnership extends SolanaCommand {
  static id = `store:transfer_store_ownership`
  static category = CONTRACT_LIST.STORE

  static examples = [
    `yarn gauntlet store:transfer_store_ownership --network=devnet --to=[PROPOSED_OWNER] [PROGRAM_STATE]`,
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.to, 'Please provide flags with "to"')
    this.requireArgs('Please provide the state as an arg!')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const state = new PublicKey(this.args[0])
    const proposedOwner = new PublicKey(this.flags.to)

    const tx = program.instruction.transferStoreOwnership(proposedOwner, {
      accounts: {
        store: state,
        authority: signer,
      },
    })

    return [tx]
  }

  execute = async () => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)
    const state = this.args[0]

    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Transferring ownership of store state (${state}) to ${this.flags.to}. Continue?`)
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
