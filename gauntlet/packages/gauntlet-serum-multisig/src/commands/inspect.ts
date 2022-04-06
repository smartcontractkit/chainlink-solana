import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, contracts } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { MULTISIG_NAME, MULTISIG_PROGRAM_ID_ENV, SCHEMA_PATH } from '../lib/constants'

export default class MultisigInspect extends SolanaCommand {
  static id = `inspect:multisig`
  static category = MULTISIG_NAME

  static examples = ['yarn gauntlet-serum-multisig multisig:inspect --network=local --state=MULTISIG_ACCOUNT']

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('state', 'Please provide multisig state address')
  }

  execute = async () => {
    const multisig = contracts.getContract(MULTISIG_NAME, '', MULTISIG_PROGRAM_ID_ENV, SCHEMA_PATH)
    const program = this.loadProgram(multisig.idl, multisig.programId.toString())

    const state = new PublicKey(this.flags.state)
    const multisigState = await program.account.multisig.fetch(state)
    const [multisigSigner] = await PublicKey.findProgramAddress([state.toBuffer()], program.programId)
    const threshold = multisigState.threshold
    const owners = multisigState.owners

    logger.info(`Multisig Info:
      - ProgramID: ${program.programId.toString()}
      - Address: ${state.toString()}
      - Signer: ${multisigSigner.toString()}
      - Threshold: ${threshold.toString()}
      - Owners: ${owners}`)

    return {
      responses: [
        {
          tx: this.wrapResponse('', state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
