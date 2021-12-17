import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey, SYSVAR_RENT_PUBKEY, Account, Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import BN from 'bn.js'

export default class ChangeThreshold extends SolanaCommand {
  static id = 'multisig:change:threshold'
  static category = CONTRACT_LIST.MULTISIG

  static examples = [
    'yarn gauntlet multisig:change:threshold --network=local --threshold=2 --approve --tx=9Vck9Gdk8o9WhxT8bgNcfJ5gbvFBN1zPuXpf8yu8o2aq --execute',
  ]

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  execute = async () => {
    // TODO: make this required
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.publicKey.toString()
    const program = this.loadProgram(multisig.idl, address)
    const data = program.coder.instruction.encode('change_threshold', {
      threshold: new BN(this.flags.threshold),
    })

    const [multisigSigner] = await PublicKey.findProgramAddress([multisigAddress.toBuffer()], program.programId)

    const accounts = [
      {
        pubkey: multisigAddress,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: multisigSigner,
        isWritable: false,
        isSigner: true,
      },
    ]

    let txPublicKey: PublicKey
    if (this.flags.tx != null) {
      txPublicKey = new PublicKey(this.flags.tx)
    } else {
      const txSize = 1000
      const transaction = Keypair.generate()
      txPublicKey = transaction.publicKey
      logger.info(`TX Account: ${txPublicKey}`)
      const tx = await program.rpc.createTransaction(program.programId, accounts, data, {
        accounts: {
          multisig: multisigAddress,
          transaction: txPublicKey,
          proposer: this.wallet.payer.publicKey,
          rent: SYSVAR_RENT_PUBKEY,
        },
        instructions: [await program.account.transaction.createInstruction(transaction, txSize)],
        signers: [transaction, this.wallet.payer],
      })
    }
    const txAccount = await program.account.transaction.fetch(txPublicKey)

    if (this.flags.approve != null) {
      const approveTx = await program.rpc.approve({
        accounts: {
          multisig: multisigAddress,
          transaction: txPublicKey,
          owner: this.wallet.publicKey,
        },
      })
    }

    if (this.flags.execute != null) {
      await program.rpc.executeTransaction({
        accounts: {
          multisig: multisigAddress,
          multisigSigner,
          transaction: txPublicKey,
        },
        remainingAccounts: txAccount.accounts
          .map((t: any) => {
            if (t.pubkey.equals(multisigSigner)) {
              return { ...t, isSigner: false }
            }
            return t
          })
          .concat({
            pubkey: txAccount.programId,
            isWritable: false,
            isSigner: false,
          }),
      })
    }

    return {
      responses: [
        {
          tx: this.wrapResponse('to be filled', 'to be filled'),
          contract: 'to be filled',
        },
      ],
    } as Result<TransactionResponse>
  }
}
