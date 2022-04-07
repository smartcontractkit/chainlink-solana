import { SolanaCommand, TransactionResponse, contracts } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { Result } from '@chainlink/gauntlet-core'
import { BN, logger } from '@chainlink/gauntlet-core/dist/utils'
import { CONTRACT_LIST, getContract } from '../lib/contracts'

export default class SetThreshold extends SolanaCommand {
  static id = 'serum_multisig:change_threshold'
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet-serum-multisig multisig:change_threshold --network=local --threshold=2 [OWNERS...]']

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG)
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
