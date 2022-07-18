import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/solana-gauntlet'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../lib/contracts'

export default class MultisigInspect extends SolanaCommand {
  static id = `serum_multisig:inspect`
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet serum_multisig:inspect --network=local --state=MULTISIG_ACCOUNT']

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('state', 'Please provide multisig state address')
  }

  execute = async () => {
    const multisig = getContract(CONTRACT_LIST.MULTISIG)
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
