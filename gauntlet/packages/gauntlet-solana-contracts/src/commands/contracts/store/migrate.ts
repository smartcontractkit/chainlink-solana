import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import {
  AccountMeta,
  PublicKey,
  TransactionInstruction,
  SYSVAR_RENT_PUBKEY,
  SYSVAR_CLOCK_PUBKEY,
} from '@solana/web3.js'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { encodeInstruction, makeTx } from '../../../lib/utils'
import { UPGRADEABLE_BPF_LOADER_PROGRAM_ID } from '../../../lib/constants'
import { makeRawUpgradeTransaction } from '../../abstract/upgrade'

type Input = {
  transmissions: string
  store: string
}

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
    const tx = makeTx(rawTx)
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
