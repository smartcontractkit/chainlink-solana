import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { CONTRACT_LIST, getContract } from '../lib/contracts'

export default class Approve extends SolanaCommand {
  static id = 'serum_multisig:approve'
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet serum_multisig:approve --network=local [IDS...]']

  constructor(flags, args) {
    super(flags, args)
  }
  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG)
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)

    logger.info(`Approving transactions: ${this.args}`)

    // map ids over this
    const ixs = await Promise.all(
      this.args.map((tx) =>
        program.methods
          .approve()
          .accounts({
            multisig: multisigAddress,
            transaction: new PublicKey(tx),
            owner: signer,
          })
          .instruction(),
      ),
    )

    return ixs
  }

  //execute not needed, this command cannot be ran outside of multisig
  execute = async () => {
    return {} as Result<TransactionResponse>
  }
}
