import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { getRDD } from '../../../lib/rdd'

type Input = {
  oracles: {
    signer: string
    transmitter: string
  }[]
  f: number | string
}
export default class SetConfig extends SolanaCommand {
  static id = 'ocr2:set_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const aggregator = rdd.contracts[this.flags.state]
    const aggregatorOperators: string[] = aggregator.oracles.map((o) => o.operator)
    const oracles = aggregatorOperators.map((operator) => ({
      // Same here
      transmitter: rdd.operators[operator].nodeAddress[0],
      // Signer should be onchainPublicKey. Check if we can support it with latest RDD changes
      signer: rdd.operators[operator].ocrSigningAddress[0].replace('0x', ''),
    }))
    const f = aggregator.config.f
    return {
      oracles,
      f,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.require(!!ocr2.programId, 'OCR 2 Program ID is necessary. Set it with "OCR2" env var')
    const address = ocr2.programId!.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const input = this.makeInput(this.flags.input)

    const owner = this.wallet.payer

    console.log(`Setting config on ${state.toString()}...`)

    // TODO: Check valid keys
    const oracles = input.oracles.map(({ signer, transmitter }) => ({
      signer: Buffer.from(signer, 'hex'),
      transmitter: new PublicKey(transmitter),
    }))
    const f = new BN(input.f)

    // Must be = oracles.length > 3 * threshold
    // TODO: Check here too
    // MAX oracles = 19
    const tx = await program.rpc.setConfig(oracles, f, {
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
