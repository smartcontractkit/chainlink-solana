import { Result } from '@chainlink/gauntlet-core'
import { createHash } from 'crypto'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { utils } from '@project-serum/anchor'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import ProposeOffchainConfig, { OffchainConfig } from '../proposeOffchainConfig'
import { serializeOffchainConfig } from '../../../../lib/encoding'
import RDD from '../../../../lib/rdd'

type Input = {
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
      version: 2,
      f,
      oracles,
      offchainConfig,
      randomSecret: this.flags.secret,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.proposalId, 'Please provide flags with "proposalId"')
    this.requireArgs('Please provide an aggregator address as argument')
    this.require(!!this.flags.secret, 'Please provide flags with "secret"')
    this.require(!!process.env.SECRET, 'Please set the SECRET env var')
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
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.args[0])
    const proposal = new PublicKey(this.flags.proposalId)
    const input = this.makeInput(this.flags.input)

    const data = await program.account.state.fetch(state)
    const proposalInfo = (await program.account.proposal.fetch(proposal)) as Proposal

    const offchainDigest = this.calculateProposalDigest(
      await this.makeDigestInput(input, new PublicKey(data.config.tokenMint)),
    )
    const isSameDigest =
      Buffer.compare(this.calculateProposalDigest(this.makeDigestInputFromProposal(proposalInfo)), offchainDigest) === 0

    this.require(isSameDigest, 'Digest generated is different from the onchain digest')
    logger.success('Generated configuration matches with onchain proposal configuration')

    const stateInfo = await program.account.state.fetch(state)
    const tokenVault = new PublicKey(stateInfo.config.tokenVault)
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )

    const payees = stateInfo.oracles.xs
      .slice(0, stateInfo.oracles.len)
      .map((oracle) => ({ pubkey: oracle.payee, isWritable: true, isSigner: false }))

    const tx = program.instruction.acceptProposal(offchainDigest, {
      accounts: {
        state: state,
        proposal: proposal,
        receiver: signer,
        authority: signer,
        tokenVault: tokenVault,
        vaultAuthority: vaultAuthority,
        tokenProgram: TOKEN_PROGRAM_ID,
      },
      remainingAccounts: payees,
    })

    return [tx]
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Continue accepting proposal of proposal ${this.flags.proposalId} on aggregator ${this.args[0]}?`)
    const txhash = await this.sendTxWithIDL(this.signAndSendRawTx, program.idl)(rawTx)
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
