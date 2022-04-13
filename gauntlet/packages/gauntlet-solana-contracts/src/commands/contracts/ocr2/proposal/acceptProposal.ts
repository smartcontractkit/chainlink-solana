import { Result } from '@chainlink/gauntlet-core'
import { createHash } from 'crypto'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import ProposeOffchainConfig from '../proposeOffchainConfig'
import { serializeOffchainConfig, deserializeConfig } from '../../../../lib/encoding'
import { prepareOffchainConfigForDiff } from '../proposeOffchainConfig'
import RDD from '../../../../lib/rdd'
import { printDiff } from '../../../../lib/diff'
import { OffchainConfig } from '../types'

type Input = {
  proposalId: string
  version: number
  f: number
  oracles: {
    transmitter: string
    signer: string
    payee: string
  }[]
  offchainConfig: OffchainConfig
  randomSecret: string
}

type ContractInput = {
  offchainDigest: Buffer
  payees: {
    pubkey: PublicKey
    isWritable: boolean
    isSigner: boolean
  }[]
  tokenVault: PublicKey
  vaultAuthority: PublicKey
}

type DigestInput = {
  version: BN
  f: BN
  tokenMint: PublicKey
  oracles: {
    transmitter: PublicKey
    signer: Buffer
    payee: PublicKey
  }[]
  offchainConfig: Buffer
}

type Proposal = {
  owner: PublicKey
  state: number
  f: number
  tokenMint: PublicKey
  oracles: {
    xs: {
      transmitter: PublicKey
      signer: {
        key: Buffer
      }
      payee: PublicKey
    }[]
    len: number
  }
  offchainConfig: {
    version: number
    xs: Buffer
    len: number
  }
}

export default class AcceptProposal extends SolanaCommand {
  static id = 'ocr2:accept_proposal'
  static category = CONTRACT_LIST.OCR_2
  static examples = [
    'yarn gauntlet ocr2:accept_proposal --network=devnet --proposalId=<PROPOSAL_ID> --rdd=<PATH_TO_RDD> <AGGREGATOR_ADDRESS>',
  ]

  input: Input
  contractInput: ContractInput

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = RDD.load(this.flags.network, this.flags.rdd)
    const aggregator = rdd.contracts[this.args[0]]

    const _toHex = (a: string) => Buffer.from(a, 'hex')
    const aggregatorOperators: any[] = aggregator.oracles.map((o) => rdd.operators[o.operator])
    const oracles = aggregatorOperators
      .map((operator) => ({
        transmitter: operator.ocrNodeAddress[0],
        signer: operator.ocr2OnchainPublicKey[0].replace('ocr2on_solana_', ''),
        payee: operator.adminAddress,
      }))
      .sort((a, b) => Buffer.compare(_toHex(a.signer), _toHex(b.signer)))

    const offchainConfig = ProposeOffchainConfig.makeInputFromRDD(rdd, this.args[0])

    const f = aggregator.config.f

    return {
      proposalId: this.flags.proposalId || this.flags.configProposal,
      version: 2,
      f,
      oracles,
      offchainConfig,
      randomSecret: this.flags.secret,
    }
  }

  makeContractInput = async (input: Input): Promise<ContractInput> => {
    const state = new PublicKey(this.args[0])
    const contractState = await this.program.account.state.fetch(state)
    const offchainDigest = this.calculateProposalDigest(
      await this.makeDigestInput(input, new PublicKey(contractState.config.tokenMint)),
    )

    const tokenVault = new PublicKey(contractState.config.tokenVault)
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      this.program.programId,
    )

    const payees = contractState.oracles.xs
      .slice(0, contractState.oracles.len.toNumber())
      .map((oracle) => ({ pubkey: oracle.payee, isWritable: true, isSigner: false }))

    return {
      offchainDigest,
      tokenVault,
      vaultAuthority,
      payees,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(
      !!this.flags.proposalId || !!this.flags.configProposal,
      'Please provide Config Proposal ID with flag "proposalId" or "configProposal"',
    )
    this.requireArgs('Please provide an aggregator address as argument')
    this.require(!!this.flags.secret, 'Please provide flags with "secret"')
    this.require(!!process.env.SECRET, 'Please set the SECRET env var')
  }

  buildCommand = async (flags, args) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    this.program = this.loadProgram(ocr2.idl, ocr2.programId.toString())
    this.input = this.makeInput(flags.input)
    this.contractInput = await this.makeContractInput(this.input)

    return this
  }

  makeDigestInputFromProposal = (proposalInfo: Proposal): DigestInput => {
    const oracles = proposalInfo.oracles.xs
      .map((oracle) => {
        return {
          transmitter: new PublicKey(oracle.transmitter),
          signer: Buffer.from(oracle.signer.key),
          payee: new PublicKey(oracle.payee),
        }
      })
      .slice(0, proposalInfo.oracles.len)
    return {
      version: new BN(proposalInfo.offchainConfig.version),
      f: new BN(proposalInfo.f),
      tokenMint: new PublicKey(proposalInfo.tokenMint),
      oracles,
      offchainConfig: proposalInfo.offchainConfig.xs.slice(0, proposalInfo.offchainConfig.len),
    }
  }

  makeDigestInput = async (input: Input, tokenMint: PublicKey): Promise<DigestInput> => {
    return {
      version: new BN(2),
      f: new BN(input.f),
      tokenMint,
      oracles: input.oracles.map((oracle) => {
        return {
          transmitter: new PublicKey(oracle.transmitter),
          signer: Buffer.from(oracle.signer, 'hex'),
          payee: new PublicKey(oracle.payee),
        }
      }),
      offchainConfig: (await serializeOffchainConfig(input.offchainConfig, process.env.SECRET!, input.randomSecret))
        .offchainConfig,
    }
  }

  calculateProposalDigest = (input: DigestInput): Buffer => {
    const hasher = input.oracles.reduce((hasher, oracle) => {
      return hasher.update(oracle.signer).update(oracle.transmitter.toBuffer()).update(oracle.payee.toBuffer())
    }, createHash('sha256').update(Buffer.from([input.oracles.length])))

    const offchainConfigHeader = Buffer.alloc(8 + 4)
    offchainConfigHeader.writeBigUInt64BE(BigInt(input.version.toNumber()), 0)
    offchainConfigHeader.writeUInt32BE(input.offchainConfig.length, 8)

    return hasher
      .update(Buffer.from([input.f.toNumber()]))
      .update(input.tokenMint.toBuffer())
      .update(offchainConfigHeader)
      .update(Buffer.from(input.offchainConfig))
      .digest()
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const tx = this.program.instruction.acceptProposal(this.contractInput.offchainDigest, {
      accounts: {
        state: new PublicKey(this.args[0]),
        proposal: new PublicKey(this.input.proposalId),
        receiver: signer,
        authority: signer,
        tokenVault: this.contractInput.tokenVault,
        vaultAuthority: this.contractInput.vaultAuthority,
        tokenProgram: TOKEN_PROGRAM_ID,
      },
      remainingAccounts: this.contractInput.payees,
    })

    return [tx]
  }

  beforeExecute = async () => {
    const contractState = await this.program.account.state.fetch(new PublicKey(this.args[0]))
    const proposalState = await this.program.account.proposal.fetch(new PublicKey(this.input.proposalId))

    const [contractConfig, proposalConfig] = [contractState, proposalState].map((state) => {
      const oracles = state.oracles?.xs.slice(0, state.oracles.len.toNumber())
      const oraclesForDiff = oracles.map(({ signer, transmitter, payee }) => ({
        signer: Buffer.from(signer.key).toString('hex'),
        transmitter: transmitter.toString(),
        payee: payee.toString(),
      }))
      const offchainConfig = deserializeConfig(
        Buffer.from(state.offchainConfig.xs).slice(0, state.offchainConfig.len.toNumber()),
      )
      const offchainConfigForDiff = prepareOffchainConfigForDiff(offchainConfig)
      return {
        f: state.f || state.config.f,
        offchainConfig: offchainConfigForDiff,
        oracles: oraclesForDiff,
      }
    })

    // verify is proposal digest correspods to the input
    const proposalDigest = this.calculateProposalDigest(this.makeDigestInputFromProposal(proposalState as Proposal))
    const isSameDigest = Buffer.compare(this.contractInput.offchainDigest, proposalDigest) === 0
    this.require(isSameDigest, 'Digest generated is different from the onchain digest')
    logger.success('Generated configuration matches with onchain proposal configuration')

    // final diff between proposal and actual contract state
    logger.info(`OffchainConfig difference in contract ${this.args[0]} and proposal ${this.input.proposalId}`)
    printDiff(contractConfig, proposalConfig)

    await prompt('Accept config proposal?')
  }

  execute = async () => {
    await this.buildCommand(this.flags, this.args)

    const signer = this.wallet.publicKey
    await this.beforeExecute()

    const rawTx = await this.makeRawTransaction(signer)
    await this.simulateTx(signer, rawTx)
    await prompt(`Continue accepting proposal of proposal ${this.input.proposalId} on aggregator ${this.args[0]}?`)

    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, this.program.idl)(rawTx)
    logger.success(`Accepted proposal on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.flags.state),
          contract: this.flags.state,
        },
      ],
    } as Result<TransactionResponse>
  }
}
