import { SolanaCommand, TransactionResponse, contracts } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { Result } from '@chainlink/gauntlet-core'
import { BN, logger } from '@chainlink/gauntlet-core/dist/utils'
import { MULTISIG_NAME, MULTISIG_PROGRAM_ID_ENV, SCHEMA_PATH } from '../lib/constants'

export default class SetThreshold extends SolanaCommand {
  static id = 'change_threshold'
  static category = MULTISIG_NAME

  static examples = ['yarn gauntlet-serum-multisig multisig:change_threshold --network=local --threshold=2 [OWNERS...]']

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = contracts.getContract(MULTISIG_NAME, '', MULTISIG_PROGRAM_ID_ENV, SCHEMA_PATH)
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)

    const threshold = new BN(this.flags.threshold)
    logger.info(`Generating data for new threshold: ${threshold.toNumber()}`)

    const ix = program.instruction.changeThreshold(threshold, {
      accounts: {
        multisig: multisigAddress,
        multisigSigner: signer,
      },
    })

    return [ix]
  }

  //execute not needed, this command cannot be ran outside of multisig
  execute = async () => {
    return {} as Result<TransactionResponse>
  }
}
