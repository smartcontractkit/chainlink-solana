import { SolanaCommand, RawTransaction, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { Result } from '@chainlink/gauntlet-core'
import { BN, logger } from '@chainlink/gauntlet-core/dist/utils'

export default class SetThreshold extends SolanaCommand {
  static id = 'set_threshold'
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet-serum-multisig set_threshold --network=local --threshold=2']

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)

    const threshold = new BN(this.flags.threshold)
    logger.info(`Generating data for new threshold: ${threshold.toNumber()}`)

    const data = program.coder.instruction.encode('change_threshold', {
      threshold,
    })

    const accounts = [
      {
        pubkey: multisigAddress,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: signer,
        isWritable: false,
        isSigner: true,
      },
    ]
    const rawTx: RawTransaction = {
      data,
      accounts,
      programId: multisig.programId,
    }

    return [rawTx]
  }

  //execute not needed, this command cannot be ran outside of multisig
  execute = async () => {
    return {} as Result<TransactionResponse>
  }
}
