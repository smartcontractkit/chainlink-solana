import { Result } from '@chainlink/gauntlet-core'
import { io, logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetConfig extends SolanaCommand {
  static id = 'ocr2:set_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:set_config --network=devnet --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (): Operator[] => {
    if (this.flags.input) return this.flags.input as Operator[]
    return []
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const owner = this.wallet.payer

    console.log(`Setting config on ${state.toString()}...`)

    const oracles = [
      {
        signer: Buffer.from('some_address'),
        transmitter: Keypair.generate().publicKey,
      },
      {
        signer: Buffer.from('some_address_2'),
        transmitter: Keypair.generate().publicKey,
      },
      {
        signer: Buffer.from('some_address_3'),
        transmitter: Keypair.generate().publicKey,
      },
      {
        signer: Buffer.from('some_address_4'),
        transmitter: Keypair.generate().publicKey,
      },
    ]
    const threshhold = 1 // rdd.config.maxFaultyNodeCount

    // oracles.length > 3 * threshold
    const tx = await program.rpc.setConfig(oracles, threshhold, {
      accounts: {
        state: state,
        authority: owner.publicKey,
      },
    })

    logger.success(`Config set on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString(), { state: state.toString() }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
