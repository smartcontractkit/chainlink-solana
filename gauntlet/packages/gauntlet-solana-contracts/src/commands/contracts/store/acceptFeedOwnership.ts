import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export default class AcceptFeedOwnership extends SolanaCommand {
  static id = 'store:accept_feed_ownership'
  static category = CONTRACT_LIST.STORE

  static examples = [`yarn gauntlet store:accept_feed_ownership --network=devnet --state=[PROGRAM_STATE] --to=[PROPOSED_OWNER]`]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
    this.require(!!this.flags.to, 'Please provide flags with "to"')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const state = new PublicKey(this.flags.state)
    const proposed = new PublicKey(this.flags.to)

    const tx = program.instruction.acceptFeedOwnership({
      accounts: {
        feed: state,
        proposedOwner: proposed,
        authority: signer,
      },
    });

    const rawTx: RawTransaction = {
      data: tx.data,
      accounts: tx.keys,
      programId: tx.programId,
    }

    return [rawTx]
  }

  execute = async () => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Accepting ownership of feed/transmission state (${this.flags.state.toString()}). Continue?`)
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
