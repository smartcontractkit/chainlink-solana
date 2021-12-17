import { SolanaCommand } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import AbstractTransaction from './abstractTransaction'
import { SolanaRawTransaction } from './abstractTransaction'

import BN from 'bn.js'

export default class ChangeOwners extends SolanaCommand {
  static id = 'multisig:set:owners'
  static category = CONTRACT_LIST.MULTISIG

  static examples = [
    'yarn gauntlet multisig:set:owners --network=local --approve --tx=9Vck9Gdk8o9WhxT8bgNcfJ5gbvFBN1zPuXpf8yu8o2aq --execute AGnZeMWkdyXBiLDG2DnwuyGSviAbCGJXyk4VhvP9Y51M QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs',
  ]

  constructor(flags, args) {
    super(flags, args)
  }
  execute = async () => {
    // TODO: make this required
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.publicKey.toString()
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
    const rawTx: SolanaRawTransaction = {
      data,
      accounts,
      programId: multisig.programId.publicKey,
    }
    const cmd = new AbstractTransaction({ ...this.flags, rawTx }, [])
    await cmd.invokeMiddlewares(cmd, this.middlewares)
    return cmd.execute()
  }
}
