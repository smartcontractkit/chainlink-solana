import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { SolanaConstructor } from '../../../lib/types'

export default class TransferFeedOwnership extends SolanaCommand {
  static id = 'store:transfer_feed_ownership'
  static category = CONTRACT_LIST.STORE

  static examples = [
    `yarn gauntlet store:transfer_feed_ownership --network=devnet --state=[FEED_STATE] --to=[PROPOSED_OWNER]`,
  ]

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
    const proposedOwner = new PublicKey(this.flags.to)

    const data = program.coder.instruction.encode('transfer_feed_ownership', {
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
        isSigner: false,
        isWritable: false,
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
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Transferring ownership of feed/transmission state (${this.flags.state.toString()}) to ${this.flags.to.toString()}. Continue?`)
    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx)
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
