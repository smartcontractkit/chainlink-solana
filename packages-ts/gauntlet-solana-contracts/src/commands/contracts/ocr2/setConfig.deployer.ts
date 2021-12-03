import { Result } from '@chainlink/gauntlet-core'
import { io, logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Keypair, PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

type Operator = {
  NodeAddress: string
  OCRConfigPublicKey: string
  OCROnchainPublicKey: string
  OCROffchainPublicKey: string
  P2PID: string
  payee: string
}

export const Nanosecond = new BN(1),
  Microsecond = Nanosecond.mul(new BN(1000)),
  Millisecond = Microsecond.mul(new BN(1000)),
  Second = Millisecond.mul(new BN(1000)),
  Minute = Second.mul(new BN(60)),
  Hour = Minute.mul(new BN(60))

export default class SetConfigDeployer extends SolanaCommand {
  static id = 'ocr2:set_config:deployer'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_config:deployer --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --keys=[OPERATORS]',
  ]
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
    this.require(!!this.flags.keys, 'Please provide flags with "keys"')
  }

  parseOperators = (operators: Operator[]): Operator[] => {
    try {
      const _isValidOperator = (operator: Operator): boolean => {
        const expectedKeys = [
          'NodeAddress',
          'OCRConfigPublicKey',
          'OCROnchainPublicKey',
          'OCROffchainPublicKey',
          'P2PID',
          'payeeAddress',
        ]
        return expectedKeys.every((key) => !!operator[key])
      }
      if (operators.length === 0) throw new Error('Expected list of operators')
      const validOperators = operators.every(_isValidOperator)
      // if (!validOperators) throw new Error('Unexpected operators format')
      return operators
    } catch (e) {
      logger.error(`Error parsing operators: ${e.message}`)
      return []
    }
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const operators: Operator[] = this.parseOperators(this.flags.keys)
    this.require(operators.length > 0, 'Provide valid operators')

    const state = new PublicKey(this.flags.state)

    const version = this.flags.version || 1
    const offchainConfigVersion = new BN(version)
    const threshhold = new BN(this.flags.threshhold || 1)

    const signers = operators.map(({ OCROnchainPublicKey, NodeAddress }) => ({
      signer: Buffer.from(OCROnchainPublicKey),
      transmitter: new PublicKey(NodeAddress),
    }))
    const max = new BN(this.flags.max || '10000000000').toBuffer()
    const min = new BN(this.flags.min || 0).toBuffer()
    const onchainConfig = Buffer.from([1, ...max, ...min])

    // TODO: set offchain config details
    // const offchainConfig = {
    //   deltaProgress: Second.mul(new BN(2)),
    //   DeltaResend: Second.mul(new BN(5)),
    //   DeltaRound: Second.mul(new BN(1)),
    //   DeltaGrace: Millisecond.mul(new BN(500)),
    //   DeltaStage: Second.mul(new BN(5)),
    //   rMax: 3,
    //   s: operators.length,
    //   offchainPublicKeys: operators.map(({ OCROffchainPublicKey }) => OCROffchainPublicKey),
    //   peerIds: operators.map(({ P2PID }) => P2PID),
    //   MaxDurationQuery: Millisecond.mul(new BN(500)),
    //   MaxDurationObservation: Millisecond.mul(new BN(500)),
    //   MaxDurationReport: Millisecond.mul(new BN(500)),
    //   MaxDurationShouldAcceptFinalizedReport: Millisecond.mul(new BN(500)),
    //   MaxDurationShouldTransmitAcceptedReport: Millisecond.mul(new BN(500)),
    // }

    console.log(`Setting config on ${state.toString()}...`)
    const tx = await program.rpc.setConfig(
      signers,
      threshhold,
      onchainConfig,
      offchainConfigVersion,
      Buffer.from([1, 2, 3]),
      {
        accounts: {
          state: state,
          authority: this.wallet.payer.publicKey,
        },
        signers: [this.wallet.payer],
      },
    )

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
