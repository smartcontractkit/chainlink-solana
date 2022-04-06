import { SolanaCommand, TransactionResponse, contracts } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { MULTISIG_NAME, MULTISIG_PROGRAM_ID_ENV, SCHEMA_PATH } from '../lib/constants'

export default class SetOwners extends SolanaCommand {
  static id = 'set_owners'
  static category = MULTISIG_NAME

  static examples = ['yarn gauntlet-serum-multisig multisig:set_owners --network=local']

  constructor(flags, args) {
    super(flags, args)
  }
  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = contracts.getContract(MULTISIG_NAME, '', MULTISIG_PROGRAM_ID_ENV, SCHEMA_PATH)
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)

    const owners = this.args.map((a) => new PublicKey(a))

    logger.info(`Generating data for new owners: ${owners.map((o) => o.toString())}`)

    const ix = program.instruction.setOwners(owners, {
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
