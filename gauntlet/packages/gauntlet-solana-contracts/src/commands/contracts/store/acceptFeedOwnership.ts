import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class AcceptFeedOwnership extends SolanaCommand {
  static id = 'store:accept_feed_ownership'
  static category = CONTRACT_LIST.STORE

  static examples = [`yarn gauntlet store:accept_feed_ownership --network=devnet [PROGRAM_STATE]`]

  constructor(flags, args) {
    super(flags, args)

    this.requireArgs('Please provide the state as an arg!')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const state = new PublicKey(this.args[0])

    // Need to resolve feed.proposedOwner. This will either match signer
    // store with store.owner == signer. If not, the instruction will error
    const feedAccount = await program.account.transmissions.fetch(state)

    const tx = program.instruction.acceptFeedOwnership({
      accounts: {
        feed: state,
        proposedOwner: feedAccount.proposedOwner,
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
    await prompt(`Accepting ownership of feed/transmission state (${state}). Continue?`)
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
