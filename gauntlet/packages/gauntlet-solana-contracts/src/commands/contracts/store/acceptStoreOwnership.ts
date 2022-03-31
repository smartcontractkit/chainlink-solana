import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export default class AcceptStoreOwnership extends SolanaCommand {
  static id = `store:accept_store_ownership`
  static category = CONTRACT_LIST.STORE

  static examples = [`yarn gauntlet store:accept_store_ownership --network=devnet [PROGRAM_STATE]`]

  constructor(flags, args) {
    super(flags, args)

    this.requireArgs('Please provide the state as an arg!')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const state = new PublicKey(this.args[0])

    const tx = program.instruction.acceptStoreOwnership({
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
    await prompt(`Accepting ownership of store state (${state}). Continue?`)
    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx)
    logger.success(`Accepted ownership on tx hash: ${txhash}`)
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
