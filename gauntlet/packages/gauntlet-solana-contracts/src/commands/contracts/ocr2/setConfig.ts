import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { ORACLES_MAX_LENGTH } from '../../../lib/constants'
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
    const aggregatorOperators: any[] = aggregator.oracles.map((o) => rdd.operators[o.operator])
    const oracles = aggregatorOperators.map((operator) => ({
      // Same here
      transmitter: operator.ocrNodeAddress[0],
      signer: operator.ocr2OnchainPublicKey[0].replace('ocr2on_solana_', ''),
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
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const input = this.makeInput(this.flags.input)

    const owner = this.wallet.payer

    const oracles = input.oracles.map(({ signer, transmitter }) => ({
      signer: Buffer.from(signer, 'hex'),
      transmitter: new PublicKey(transmitter),
    }))
    const f = new BN(input.f)

    const minOracleLength = f.mul(new BN(3)).toNumber()
    this.require(oracles.length > minOracleLength, `Number of oracles should be higher than ${minOracleLength}`)
    this.require(
      oracles.length <= ORACLES_MAX_LENGTH,
      `Oracles max length is ${ORACLES_MAX_LENGTH}, currently ${oracles.length}`,
    )

    logger.log('Config information:', input)
    await prompt(`Continue setting config on ${state.toString()}?`)

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
