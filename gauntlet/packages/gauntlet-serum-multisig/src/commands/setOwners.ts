import { SolanaCommand, RawTransaction, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'

export default class SetOwners extends SolanaCommand {
  static id = 'set_owners'
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet-serum-multisig set_owners --network=local']

  constructor(flags, args) {
    super(flags, args)
  }
  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)

    const owners = this.args.map((a) => new PublicKey(a))

    logger.info(`Generating data for new owners: ${owners.map((o) => o.toString())}`)

    const data = program.coder.instruction.encode('set_owners', {
      owners,
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
