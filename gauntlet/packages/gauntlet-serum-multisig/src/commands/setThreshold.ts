import { SolanaCommand, RawTransaction, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { Result } from '@chainlink/gauntlet-core'

import BN from 'bn.js'

export default class SetThreshold extends SolanaCommand {
  static id = 'set:threshold'
  static category = CONTRACT_LIST.MULTISIG

  static examples = [
    'yarn gauntlet-serum-multisig set:threshold --network=local --threshold=2 --approve --tx=9Vck9Gdk8o9WhxT8bgNcfJ5gbvFBN1zPuXpf8yu8o2aq --execute',
  ]

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const multisigAddress = new PublicKey(process.env.MULTISIG_ADDRESS || '')
    const multisig = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = multisig.programId.toString()
    const program = this.loadProgram(multisig.idl, address)
    const data = program.coder.instruction.encode('change_threshold', {
      threshold: new BN(this.flags.threshold),
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
