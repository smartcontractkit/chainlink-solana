import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

// TODO: Depends on RDD info
export default class SetConfig extends SolanaCommand {
  static id = 'ocr2:set_config'
  static category = CONTRACT_LIST.ACCESS_CONTROLLER

  static examples = [
    'yarn gauntlet ocr2:set_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:set_config --network=devnet --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const address = this.args[0] || process.env.OCR_2
    this.require(!!address, 'Provide an OCR 2 program address')
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, address!)

    const state = new PublicKey(this.flags.state)

    console.log(`Setting config on ${state.toString()}...`)

    const version = 1
    const oracles = []
    const threshhold = 2
    const onchainConfig = {}
    const offchainConfigVersion = new BN(version)
    const offchainConfig = {}

    const tx = await program.rpc.setConfig(oracles, threshhold, onchainConfig, offchainConfigVersion, offchainConfig, {
      accounts: {
        state: state,
        authority: this.wallet.payer.publicKey,
      },
      signers: [this.wallet.payer],
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
