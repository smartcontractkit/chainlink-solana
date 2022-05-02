import { Result } from '@chainlink/gauntlet-core'
import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, utils, logger } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey, TransactionInstruction } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { makeRawUpgradeTransaction } from '../../abstract/upgrade'

export default class Migrate extends SolanaCommand {
  static id = 'store:migrate'
  static category = CONTRACT_LIST.STORE

  static examples = [
    'yarn gauntlet store:migrate --network=devnet --state=[STORE_ACCOUNT] --buffer=[BUFFER_ACCOUNT] [TRANSMISSIONS ACCOUNTS...]',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state" for store account')
    this.require(!!this.flags.buffer, 'Please provide flags with "buffer" for program buffer upgrade account')
    this.require(this.args.length != 0, 'Please provide the transmissions accounts to upgrade')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const store = getContract(CONTRACT_LIST.STORE, '')
    const storeAddress = store.programId.toString()
    const storeProgram = this.loadProgram(store.idl, storeAddress)

    const storeState = new PublicKey(this.flags.state)
    // build upgrade transaction
    const upgradeRawTx: TransactionInstruction[] = await makeRawUpgradeTransaction(
      signer,
      CONTRACT_LIST.STORE,
      this.flags.buffer,
    )

    // build transmission migrate instructions
    const transmissionAccounts: AccountMeta[] = this.args.map((a: string) => ({
      pubkey: new PublicKey(a),
      isSigner: false,
      isWritable: true,
    }))
    const migrateTx = storeProgram.instruction.migrate({
      accounts: {
        state: storeState,
        authority: signer,
      },
      remainingAccounts: transmissionAccounts,
    })

    return [...upgradeRawTx, migrateTx]
  }

  execute = async () => {
    const contract = getContract(CONTRACT_LIST.STORE, '')
    const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
    const tx = utils.makeTx(rawTx)
    logger.debug(tx)
    logger.info(
      `Migrating Store program (${contract.programId.toString()})
    - store account: ${this.flags.state}
    - buffer account: ${this.flags.buffer}`,
    )
    logger.info('for each transmission account:')
    this.args.forEach((a) => console.log(`    - ${a}`))

    await prompt(`Continue migrating?`)

    logger.loading('Sending tx...')
    const txhash = await this.sendTx(tx, [this.wallet.payer], contract.idl)
    logger.success(`Migrated on tx hash: ${txhash}`)

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
