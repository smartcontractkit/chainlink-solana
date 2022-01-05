import { SolanaCommand, RawTransaction, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { Result } from '@chainlink/gauntlet-core'

import BN from 'bn.js'

export default class SetOwners extends SolanaCommand {
  static id = 'multisig:set:owners'
  static category = CONTRACT_LIST.MULTISIG

  static examples = [
    'yarn gauntlet-serum-multisig set:owners --network=local --approve --tx=9Vck9Gdk8o9WhxT8bgNcfJ5gbvFBN1zPuXpf8yu8o2aq --execute AGnZeMWkdyXBiLDG2DnwuyGSviAbCGJXyk4VhvP9Y51M QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs',
  ]

  constructor(flags, args) {
    super(flags, args)
  }
  makeRawTransaction = async () => {
    // TODO: make this required
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)
    const data = program.coder.instruction.encode('set_owners', {
      owners: this.args.map((a) => new PublicKey(a)),
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
