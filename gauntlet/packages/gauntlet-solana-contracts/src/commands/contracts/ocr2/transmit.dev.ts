import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { parseIdlErrors, ProgramError, utils } from '@project-serum/anchor'
import { Keypair, PublicKey, Transaction, TransactionInstruction } from '@solana/web3.js'
import { createHash } from 'crypto'
import * as secp256k1 from 'secp256k1'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class Transmit extends SolanaCommand {
  static id = 'ocr2:transmit'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:transmit --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --transmissions=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --validator=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --accessController=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --round=2',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const validatorProgram = getContract(CONTRACT_LIST.DEVIATION_FLAGGING_VALIDATOR, '')

    const state = new PublicKey(this.flags.state)
    const transmissions = new PublicKey(this.flags.transmissions)
    const validator = new PublicKey(this.flags.validator)
    const accessController = new PublicKey(this.flags.accessController)
    const round = Number(this.flags.round) || 1
    const info = await program.account.state.fetch(state)

    const reportContext: any[] = []
    reportContext.push(...info.config.latestConfigDigest)
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)
    reportContext.push(0, 0, 0) // 27 byte padding
    reportContext.push(0, 0, 0, round) // epoch 1
    reportContext.push(round) //  round 1
    // extra_hash 32 bytes
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)
    reportContext.push(0, 0, 0, 0, 0, 0, 0, 0)

    const rawReport: any[] = []
    rawReport.push(97, 91, 43, 83) // observations_timestamp
    rawReport.push(0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // observers
    rawReport.push(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210) // median
    rawReport.push(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2) // juels per lamport (2)

    let hash = createHash('sha256').update(Buffer.from(rawReport)).update(Buffer.from(reportContext)).digest()

    const OPERATORS: any[] = []
    const rawSignatures: any[] = []
    for (let oracle of OPERATORS.slice(0, 3 * info.config.f + 1)) {
      const { signature, recid } = secp256k1.ecdsaSign(hash, Buffer.from(oracle.signer.secretKey))
      rawSignatures.push(...signature)
      rawSignatures.push(recid)
    }

    const transmitter = Keypair.fromSecretKey(Uint8Array.from(OPERATORS[0].transmitter))

    const [validatorAuthority, validatorNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('validator')), state.toBuffer()],
      program.programId,
    )

    const tx = new Transaction()
    tx.add(
      new TransactionInstruction({
        programId: program.programId,
        keys: [
          { pubkey: state, isWritable: true, isSigner: false },
          { pubkey: transmitter.publicKey, isWritable: false, isSigner: true },
          { pubkey: transmissions, isWritable: true, isSigner: false },
          { pubkey: validatorProgram.programId!, isWritable: false, isSigner: false },
          { pubkey: validator, isWritable: true, isSigner: false },
          { pubkey: validatorAuthority, isWritable: false, isSigner: false },
          { pubkey: accessController, isWritable: false, isSigner: false },
        ],
        data: Buffer.concat([
          Buffer.from([validatorNonce]),
          Buffer.from(reportContext),
          Buffer.from(rawReport),
          Buffer.from(rawSignatures),
        ]),
      }),
    )

    let txhash
    try {
      txhash = await this.provider.send(tx, [transmitter])
    } catch (err) {
      // Translate IDL error
      const idlErrors = parseIdlErrors(program.idl)
      let translatedErr = ProgramError.parse(err, idlErrors)
      if (translatedErr === null) {
        throw err
      }
      throw translatedErr
    }
    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
