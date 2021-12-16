import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey, SYSVAR_RENT_PUBKEY, Account } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

const DEFAULT_MAX_PARTICIPANTS_LENGTH = 30

export default class MultisigCreate extends SolanaCommand {
  static id = 'multisig:create'
  static category = CONTRACT_LIST.MULTISIG

  static examples = [
    'yarn gauntlet multisig:create --network=local --threshold=1 653SW42RnZ3aebVBqkHxDie4WUP6iVuHtM3nj4XoTafx 3yviqE7SeYUbiN8L4q9QcPKSiAKFN2BNqNKUTdxTTbpP',
  ]

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  execute = async () => {
    this.require(this.args.length > 0, 'Please provide at least one owner as an argument')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.publicKey.toString()
    const program = this.loadProgram(multisig.idl, address)

    const multisigAccount = new Account()

    const [, nonce] = await PublicKey.findProgramAddress([multisigAccount.publicKey.toBuffer()], program.programId)
    //TODO: check that all args are valid addresses
    const owners = this.args.map((a) => new PublicKey(a))

    const threshold = this.flags.threshold

    const maxParticipantLength = this.flags.maxParticipantLength || DEFAULT_MAX_PARTICIPANTS_LENGTH
    //TODO: copied from multisig's UI, need to see if these fit our needs
    const baseSize = 8 + 8 + 1 + 4
    // Add enough for 2 more participants, in case the user changes one's
    /// mind later.
    const fudge = 64
    // Can only grow the participant set by 2x the initialized value.
    const ownerSize = maxParticipantLength * 32 + 8
    const multisigSize = baseSize + ownerSize + fudge
    await prompt(
      `Create new multisig with owners: ${this.args}, threshold: ${this.flags.threshold} and max owners length: ${maxParticipantLength}?`,
    )
    const tx = await program.rpc.createMultisig(owners, new BN(threshold), nonce, {
      accounts: {
        multisig: multisigAccount.publicKey,
        rent: SYSVAR_RENT_PUBKEY,
      },
      signers: [multisigAccount],
      instructions: [await program.account.multisig.createInstruction(multisigAccount, multisigSize)],
    })
    logger.info(`Multisig address: ${multisigAccount.publicKey}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, multisigAccount.publicKey.toString()),
          contract: multisigAccount.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
