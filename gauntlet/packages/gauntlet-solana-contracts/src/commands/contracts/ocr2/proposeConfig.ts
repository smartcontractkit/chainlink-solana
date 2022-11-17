import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { ORACLES_MAX_LENGTH } from '../../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import RDD from '../../../lib/rdd'
import { printDiff } from '../../../lib/diff'

type Input = {
  oracles: {
    signer: string
    transmitter: string
    payee: string
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
        payee: operator.adminAddress,
      }))
      .sort((a, b) => Buffer.compare(_toHex(a.signer), _toHex(b.signer)))

    const f = aggregator.config.f

    return {
      oracles,
      f,
      proposalId: this.flags.proposalId || this.flags.configProposal,
    }
  }

  constructor(flags, args) {
    super(flags, args)
    this.require(
      !!this.flags.proposalId || !!this.flags.configProposal,
      'Please provide Config Proposal ID with flag "proposalId" or "configProposal"',
    )
    this.requireArgs('Please provide an aggregator address')
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)

    return this
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const proposal = new PublicKey(this.input.proposalId)

    const oracles = this.input.oracles.map(({ signer, transmitter }) => ({
      signer: Buffer.from(signer, 'hex'),
      transmitter: new PublicKey(transmitter),
    }))

    // proposeConfig

    const f = new BN(this.input.f)

    const minOracleLength = f.mul(new BN(3)).toNumber()
    this.require(oracles.length > minOracleLength, `Number of oracles should be higher than ${minOracleLength}`)
    this.require(
      oracles.length <= ORACLES_MAX_LENGTH,
      `Oracles max length is ${ORACLES_MAX_LENGTH}, currently ${oracles.length}`,
    )

    const configIx = await this.program.methods
      .proposeConfig(oracles, f)
      .accounts({
        proposal,
        authority: signer,
      })
      .instruction()

    // proposePayees

    const link = new PublicKey(this.flags.link || process.env.LINK)

    const payees = this.input.oracles.map(({ payee }) => ({
      pubkey: new PublicKey(payee),
      isWritable: true,
      isSigner: false,
    }))

    const payeesIx = await this.program.methods
      .proposePayees(link)
      .accounts({
        proposal,
        authority: signer,
      })
      .remainingAccounts(payees)
      .instruction()

    return [configIx, payeesIx]
  }

  beforeExecute = async () => {
    const state = new PublicKey(this.args[0])
    const contractState = (await this.program.account.state.fetch(state)) as any

    // Prepare contract config
    const contractOracles = contractState.oracles?.xs.slice(0, contractState.oracles.len.toNumber())
    const contractOraclesForDiff = contractOracles.map(({ signer, transmitter, payee }) => ({
      signer: Buffer.from(signer.key).toString('hex'),
      transmitter: transmitter.toString(),
      payee: payee.toString(),
    }))

    const contractConfig = {
      f: contractState.config.f,
      oracles: contractOraclesForDiff,
    }

    const proposedConfig = {
      f: this.input.f,
      oracles: this.input.oracles,
    }

    logger.info(`Proposed Config for contract ${this.args[0]}:`)
    printDiff(contractConfig, proposedConfig)

    await prompt('Continue?')
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)

    const signer = this.wallet.publicKey
    await this.beforeExecute()

    const rawTxs = await this.makeRawTransaction(signer)
    // simulate all transactions first, then send them
    for (const rawTx of rawTxs) {
      await this.simulateTx(signer, [rawTx])
    }

    await prompt(`Continue setting config on ${this.args[0]}?`)

    const txs: string[] = []
    for (const rawTx of rawTxs) {
      // TODO: signAndSend in parallel (proposeConfig, proposeOffchainConfig, proposePayees) via Promise.all
      const txhash = await this.signAndSendRawTx([rawTx])
      txs.push(txhash)
    }
    const txhash = txs[txs.length - 1]
    logger.success(`Config set on tx ${txhash}`)

    return {
      responses: [ // TODO: map over responses
        {
          tx: this.wrapResponse(txhash, this.args[0]),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
