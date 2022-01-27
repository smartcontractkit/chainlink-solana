import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey, SYSVAR_RENT_PUBKEY, SYSVAR_CLOCK_PUBKEY } from '@solana/web3.js'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'
import { encodeInstruction, makeTx } from '../../../lib/utils'
import { UPGRADEABLE_BPF_LOADER_PROGRAM_ID } from '../../../lib/constants'

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
    const bufferAccount = new PublicKey(this.flags.buffer)

    // build deploy buffer instruction
    const storeProgramId = new PublicKey(store.programId)
    const [programDataKey, _nonce] = await PublicKey.findProgramAddress(
      [storeProgramId.toBuffer()],
      UPGRADEABLE_BPF_LOADER_PROGRAM_ID,
    )
    const upgradeData = encodeInstruction({ Upgrade: {} })
    const upgradeAccounts: AccountMeta[] = [
      { pubkey: programDataKey, isSigner: false, isWritable: true },
      { pubkey: storeProgramId, isSigner: false, isWritable: true },
      { pubkey: bufferAccount, isSigner: false, isWritable: true },
      { pubkey: signer, isSigner: false, isWritable: true },
      { pubkey: SYSVAR_RENT_PUBKEY, isSigner: false, isWritable: false },
      { pubkey: SYSVAR_CLOCK_PUBKEY, isSigner: false, isWritable: false },
      { pubkey: signer, isSigner: true, isWritable: false },
    ]
    const upgradeRawTx: RawTransaction = {
      data: upgradeData,
      accounts: upgradeAccounts,
      programId: UPGRADEABLE_BPF_LOADER_PROGRAM_ID,
    }

    // build transmission migrate instructions
    const transmissionAccounts: AccountMeta[] = this.args.map((a: string) => ({
      pubkey: new PublicKey(a),
      isSigner: false,
      isWritable: true,
    }))
    const migrateData = storeProgram.coder.instruction.encode('migrate', {})
    const migrateAccounts: AccountMeta[] = [
      {
        pubkey: storeState,
        isSigner: false,
        isWritable: true,
      },
      {
        pubkey: signer,
        isSigner: true,
        isWritable: false,
      },
      ...transmissionAccounts,
    ]
    const migrateRawTx: RawTransaction = {
      data: migrateData,
      accounts: migrateAccounts,
      programId: storeProgram.programId,
    }

    return [upgradeRawTx, migrateRawTx]
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
    logger.info("for each transmission account:")
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
