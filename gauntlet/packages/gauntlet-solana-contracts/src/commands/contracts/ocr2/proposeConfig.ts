import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, time, prompt, longs } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey, SystemProgram } from '@solana/web3.js'
import { MAX_TRANSACTION_BYTES, ORACLES_MAX_LENGTH } from '../../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { divideIntoChunks } from '../../../lib/utils'
import { serializeOffchainConfig, deserializeConfig } from '../../../lib/encoding'
import RDD from '../../../lib/rdd'
import { printDiff } from '../../../lib/diff'

export type OffchainConfig = {
  deltaProgressNanoseconds: number
  deltaResendNanoseconds: number
  deltaRoundNanoseconds: number
  deltaGraceNanoseconds: number
  deltaStageNanoseconds: number
  rMax: number
  s: number[]
  offchainPublicKeys: string[]
  peerIds: string[]
  reportingPluginConfig: {
    alphaReportInfinite: boolean
    alphaReportPpb: number
    alphaAcceptInfinite: boolean
    alphaAcceptPpb: number
    deltaCNanoseconds: number
  }
  maxDurationQueryNanoseconds: number
  maxDurationObservationNanoseconds: number
  maxDurationReportNanoseconds: number
  maxDurationShouldAcceptFinalizedReportNanoseconds: number
  maxDurationShouldTransmitAcceptedReportNanoseconds: number
  configPublicKeys: string[]
}

const validateConfig = (input: OffchainConfig): boolean => {
  const _isNegative = (v: number): boolean => new BN(v).lt(new BN(0))
  const nonNegativeValues = [
    'deltaProgressNanoseconds',
    'deltaResendNanoseconds',
    'deltaRoundNanoseconds',
    'deltaGraceNanoseconds',
    'deltaStageNanoseconds',
    'maxDurationQueryNanoseconds',
    'maxDurationObservationNanoseconds',
    'maxDurationReportNanoseconds',
    'maxDurationShouldAcceptFinalizedReportNanoseconds',
    'maxDurationShouldTransmitAcceptedReportNanoseconds',
  ]
  for (let prop in nonNegativeValues) {
    if (_isNegative(input[prop])) throw new Error(`${prop} must be non-negative`)
  }
  const safeIntervalNanoseconds = new BN(200).mul(time.Millisecond).toNumber()
  if (input.deltaProgressNanoseconds < safeIntervalNanoseconds)
    throw new Error(
      `deltaProgressNanoseconds (${input.deltaProgressNanoseconds} ns)  is set below the resource exhaustion safe interval (${safeIntervalNanoseconds} ns)`,
    )
  if (input.deltaResendNanoseconds < safeIntervalNanoseconds)
    throw new Error(
      `deltaResendNanoseconds (${input.deltaResendNanoseconds} ns) is set below the resource exhaustion safe interval (${safeIntervalNanoseconds} ns)`,
    )

  if (input.deltaRoundNanoseconds >= input.deltaProgressNanoseconds)
    throw new Error(
      `deltaRoundNanoseconds (${input.deltaRoundNanoseconds}) must be less than deltaProgressNanoseconds (${input.deltaProgressNanoseconds})`,
    )
  const sumMaxDurationsReportGeneration = new BN(input.maxDurationQueryNanoseconds)
    .add(new BN(input.maxDurationObservationNanoseconds))
    .add(new BN(input.maxDurationReportNanoseconds))

  if (sumMaxDurationsReportGeneration.gte(new BN(input.deltaProgressNanoseconds)))
    throw new Error(
      `sum of MaxDurationQuery/Observation/Report (${sumMaxDurationsReportGeneration}) must be less than deltaProgressNanoseconds (${input.deltaProgressNanoseconds})`,
    )

  if (input.rMax <= 0 || input.rMax >= 255)
    throw new Error(`rMax (${input.rMax}) must be greater than zero and less than 255`)

  if (input.s.length >= 1000) throw new Error(`Length of S (${input.s.length}) must be less than 1000`)
  for (let i = 0; i < input.s.length; i++) {
    const s = input.s[i]
    if (s < 0 || s > ORACLES_MAX_LENGTH)
      throw new Error(`S[${i}] (${s}) must be between 0 and Max Oracles (${ORACLES_MAX_LENGTH})`)
  }

  return true
}

type Input = {
  oracles: {
    signer: string
    transmitter: string
    payee: string
  }[]
  f: number | string
  offchainConfig: OffchainConfig
  userSecret?: string
}

export const prepareOffchainConfigForDiff = (config: OffchainConfig, extra?: Object): Object => {
  return longs.longsInObjToNumbers({
    ...config,
    ...(extra || {}),
    offchainPublicKeys: config.offchainPublicKeys?.map((key) => Buffer.from(key).toString('hex')),
  }) as Object
}

const _toHex = (a: string) => Buffer.from(a, 'hex')

export default class ProposeConfig extends SolanaCommand {
  static id = 'ocr2:propose_config'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:propose_config --network=devnet --rdd=[PATH_TO_RDD] [AGGREGATOR_ADDRESS]']

  input: Input
  randomSecret: string
  proposal: Keypair

  static makeInputFromRDD = (rdd: any, stateAddress: string): OffchainConfig => {
    const aggregator = rdd.contracts[stateAddress]
    const config = aggregator.config

    const _getSigner = (o) => o.ocr2OnchainPublicKey[0].replace('ocr2on_solana_', '')
    const aggregatorOperators: any[] = aggregator.oracles
      .map((o) => rdd.operators[o.operator])
      .sort((a, b) => Buffer.compare(_toHex(_getSigner(a)), _toHex(_getSigner(b))))
    const operatorsPublicKeys = aggregatorOperators.map((o) =>
      o.ocr2OffchainPublicKey[0].replace('ocr2off_solana_', ''),
    )
    const operatorsPeerIds = aggregatorOperators.map((o) => o.peerId[0])
    const operatorConfigPublicKeys = aggregatorOperators.map((o) =>
      o.ocr2ConfigPublicKey[0].replace('ocr2cfg_solana_', ''),
    )

    const input: OffchainConfig = {
      deltaProgressNanoseconds: time.durationToNanoseconds(config.deltaProgress).toNumber(),
      deltaResendNanoseconds: time.durationToNanoseconds(config.deltaResend).toNumber(),
      deltaRoundNanoseconds: time.durationToNanoseconds(config.deltaRound).toNumber(),
      deltaGraceNanoseconds: time.durationToNanoseconds(config.deltaGrace).toNumber(),
      deltaStageNanoseconds: time.durationToNanoseconds(config.deltaStage).toNumber(),
      rMax: config.rMax,
      s: config.s,
      offchainPublicKeys: operatorsPublicKeys,
      peerIds: operatorsPeerIds,
      reportingPluginConfig: {
        alphaReportInfinite: config.reportingPluginConfig.alphaReportInfinite,
        alphaReportPpb: Number(config.reportingPluginConfig.alphaReportPpb),
        alphaAcceptInfinite: config.reportingPluginConfig.alphaAcceptInfinite,
        alphaAcceptPpb: Number(config.reportingPluginConfig.alphaAcceptPpb),
        deltaCNanoseconds: time.durationToNanoseconds(config.reportingPluginConfig.deltaC).toNumber(),
      },
      maxDurationQueryNanoseconds: time.durationToNanoseconds(config.maxDurationQuery).toNumber(),
      maxDurationObservationNanoseconds: time.durationToNanoseconds(config.maxDurationObservation).toNumber(),
      maxDurationReportNanoseconds: time.durationToNanoseconds(config.maxDurationReport).toNumber(),
      maxDurationShouldAcceptFinalizedReportNanoseconds: time
        .durationToNanoseconds(config.maxDurationShouldAcceptFinalizedReport)
        .toNumber(),
      maxDurationShouldTransmitAcceptedReportNanoseconds: time
        .durationToNanoseconds(config.maxDurationShouldTransmitAcceptedReport)
        .toNumber(),
      configPublicKeys: operatorConfigPublicKeys,
    }
    return input
  }

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = RDD.load(this.flags.network, this.flags.rdd)
    const aggregator = rdd.contracts[this.args[0]]

    const userSecret = this.flags.secret
    if (!!userSecret) {
      logger.info(`Using given random secret: ${userSecret}`)
    }

    const offchainConfig = ProposeConfig.makeInputFromRDD(rdd, this.args[0])

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
      offchainConfig,
    }
  }

  constructor(flags, args) {
    super(flags, args)
    this.requireArgs('Please provide an aggregator address')
    this.require(
      // TODO: should be able to just rely on random secret?
      !!process.env.SECRET,
      'Please specify the Gauntlet secret words e.g. SECRET="awe fluke polygon tonic lilly acuity onyx debra bound gilbert wane"',
    )
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)

    return this
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const proposal = Keypair.generate()
    this.proposal = proposal

    // createProposal
    const version = new BN(2)

    logger.log('Generating data for creating config proposal')
    logger.log('Config Proposal state will be at:', proposal.toString())

    const createIx = await this.program.methods
      .createProposal(version)
      .accounts({
        proposal: proposal.publicKey,
        authority: signer,
      })
      .instruction()
    const defaultAccountSize = new BN(this.program.account.proposal.size)
    const createAccountIx = SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: proposal.publicKey,
      space: defaultAccountSize.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(defaultAccountSize.toNumber()),
      programId: this.program.programId,
    })

    // proposeConfig

    const oracles = this.input.oracles.map(({ signer, transmitter }) => ({
      signer: Buffer.from(signer, 'hex'),
      transmitter: new PublicKey(transmitter),
    }))

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
        proposal: proposal.publicKey,
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
        proposal: proposal.publicKey,
        authority: signer,
      })
      .remainingAccounts(payees)
      .instruction()

    // proposeOffchainConfig

    const maxBufferSize = this.flags.bufferSize || MAX_TRANSACTION_BYTES
    // process.env.SECRET is required on the command
    const { offchainConfig, userSecret } = this.input
    const { offchainConfig: serializedOffchainConfig, randomSecret } = await serializeOffchainConfig(
      offchainConfig,
      process.env.SECRET!,
      userSecret,
    )
    this.randomSecret = randomSecret

    validateConfig(this.input.offchainConfig)

    logger.info(`Offchain config size: ${serializedOffchainConfig.byteLength}`)
    this.require(serializedOffchainConfig.byteLength < 4096, 'Offchain config must be lower than 4096 bytes')

    // There's a byte limit per transaction. Write the config in chunks
    const offchainConfigChunks = divideIntoChunks(serializedOffchainConfig, maxBufferSize)
    if (offchainConfigChunks.length > 1) {
      logger.info(
        `Config size (${serializedOffchainConfig.byteLength} bytes) is bigger than transaction limit. It needs to be configured using ${offchainConfigChunks.length} transactions`,
      )
    }

    const offchainConfigIxs = await Promise.all(
      offchainConfigChunks.map((buffer) =>
        this.program.methods
          .writeOffchainConfig(buffer)
          .accounts({
            proposal: proposal.publicKey,
            authority: signer,
          })
          .instruction(),
      ),
    )

    const finalizeIx = await this.program.methods
      .finalizeProposal()
      .accounts({
        proposal: proposal.publicKey,
        authority: signer,
      })
      .instruction()

    return [createAccountIx, createIx, configIx, payeesIx, ...offchainConfigIxs, finalizeIx]
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

    // Config in contract
    const contractOffchainConfig = deserializeConfig(
      Buffer.from(contractState.offchainConfig.xs).slice(0, contractState.offchainConfig.len.toNumber()),
    )
    const contractOffchainConfigForDiff = prepareOffchainConfigForDiff(contractOffchainConfig)
    const proposedConfigForDiff = prepareOffchainConfigForDiff(this.input.offchainConfig)

    logger.info(`Proposed OffchainConfig for contract ${this.args[0]}`)
    printDiff(contractOffchainConfigForDiff, proposedConfigForDiff)

    logger.info(
      `Important: Save this secret
        - If you run this command for the same configuration, use the same RANDOM SECRET
        - You will need to provide the secret to approve the config proposal
      Provide it with --secret flag`,
    )
    logger.info(`${this.randomSecret}`)
    logger.line()

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
    logger.success(`Config proposal finalized on tx ${txhash}`)
    logger.line()
    logger.info('Use the Config Proposal ID in future proposal commands:')
    logger.info(this.proposal.publicKey.toString())
    logger.line()

    return {
      data: {
        secret: this.randomSecret,
        proposal: this.proposal.publicKey.toString(),
      },
      responses: [
        // TODO: map over responses
        {
          tx: this.wrapResponse(txhash, this.args[0]),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
