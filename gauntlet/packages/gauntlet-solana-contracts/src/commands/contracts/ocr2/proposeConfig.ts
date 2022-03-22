import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt, diff } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Idl, Program } from '@project-serum/anchor'
import { PublicKey } from '@solana/web3.js'
import { ORACLES_MAX_LENGTH } from '../../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'

type Input = {
  oracles: {
    signer: string
    transmitter: string
  }[]
  f: number | string
  proposalId: string
}

const _toHex = (a: string) => Buffer.from(a, 'hex')

export default class ProposeConfig extends SolanaCommand {
  static id = 'ocr2:propose_config'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:propose_config --network=devnet --rdd=[PATH_TO_RDD] --proposalId=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC [AGGREGATOR_ADDRESS]',
  ]

  input: Input

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = RDD.load(this.flags.network, this.flags.rdd)
    const aggregator = rdd.contracts[this.args[0]]

    const aggregatorOperators: any[] = aggregator.oracles.map((o) => rdd.operators[o.operator])
    const oracles = aggregatorOperators
      .map((operator) => ({
        transmitter: operator.ocrNodeAddress[0],
        signer: operator.ocr2OnchainPublicKey[0].replace('ocr2on_solana_', ''),
      }))
      .sort((a, b) => Buffer.compare(_toHex(a.signer), _toHex(b.signer)))

    const f = aggregator.config.f

    return {
      oracles,
      f,
      proposalId: this.flags.proposalId,
    }
  }

  constructor(flags, args) {
    super(flags, args)
    this.require(!!this.flags.proposalId, 'Please provide flags with "proposalId"')
    this.requireArgs('Please provide an aggregator address')
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)

    return this
  }

  makeRawTransaction = async (signer: PublicKey, input?: Input) => {
    if (!input) {
      input = this.makeInput(this.flags.input)
    }

    const proposal = new PublicKey(input.proposalId)

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

    const ix = this.program.instruction.proposeConfig(oracles, f, {
      accounts: {
        proposal,
        authority: signer,
      },
    })

    return [ix]
  }

  beforeExecute = async () => {
    const aggregator = new PublicKey(this.args[0])
    const contractState = await this.program.account.state.fetch(aggregator)

    // Prepare contract config
    const contractOracles = contractState.oracles?.xs
      .slice(0, contractState.oracles.len.toNumber())
      .sort((a, b) => Buffer.compare(_toHex(a.signer.key), _toHex(b.signer.key)))
    const contractOraclesForDiff = contractOracles?.reduce((acc, { signer, transmitter }, idx) => {
      return {
        ...acc,
        [`oracle#${idx}`]: {
          signer: Buffer.from(signer.key).toString('hex'),
          transmitter: transmitter.toString(),
        },
      }
    }, {})

    const contractConfig = {
      f: contractState.config.f,
      oracles: contractOraclesForDiff,
    }

    const proposedConfig = {
      f: this.input.f,
      oracles: this.input.oracles.reduce((acc, oracle, idx) => {
        return { ...acc, [`oracle#${idx}`]: oracle }
      }, {}),
    }

    logger.info(`Proposing new config on contract ${this.args[0]}:`)
    diff.printDiff(contractConfig, proposedConfig)

    await prompt('Continue?')
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)

    const signer = this.wallet.publicKey
    const input = this.makeInput(this.flags.input)

    const rawTx = await this.makeRawTransaction(signer, input)
    await this.beforeExecute()
    await this.simulateTx(signer, rawTx)
    await prompt(`Continue setting config on ${this.args[0].toString()}?`)

    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Config set on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.args[0]),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
