import { Result } from '@chainlink/gauntlet-core'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { getRDD } from '../../../../lib/rdd'
import {
  inspect,
  Inspection,
  makeInspection,
  toComparableNumber,
  toComparablePubKey,
} from '../../../../core/inspection'

type Input = {
  description: string
  decimals: string | number
  minAnswer: string | number
  maxAnswer: string | number
  transmitters: string[]
  billingAccessController: string
  requesterAccessController: string
  link: string
}

export default class OCR2Inspect extends SolanaCommand {
  static id = 'ocr2:inspect'
  static category = CONTRACT_LIST.OCR_2

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    const info = rdd.contracts[this.flags.state]
    const aggregatorOperators: string[] = info.oracles.map((o) => o.operator)
    const transmitters = aggregatorOperators.map((operator) => rdd.operators[operator].nodeAddress[0])
    const billingAccessController =
      this.flags.billingAccessController || process.env.BILLING_ADMIN_ACCESS_CONTROLLER_ADDRESS
    const requesterAccessController =
      this.flags.requesterAccessController || process.env.REQUESTER_ADMIN_ACCESS_CONTROLLER_ADDRESS
    const link = this.flags.link || process.env.LINK_ADDRESS
    return {
      description: info.name,
      decimals: info.decimals,
      minAnswer: info.minSubmissionValue,
      maxAnswer: info.maxSubmissionValue,
      transmitters,
      billingAccessController,
      requesterAccessController,
      link,
    }
  }

  constructor(flags, args) {
    super(flags, args)
    this.require(!!this.flags.state, 'Please provide flags with "state""')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.publicKey.toString())

    const state = new PublicKey(this.flags.state)
    const input = this.makeInput(this.flags.input)
    const data = await program.account.state.fetch(state)

    const inspections: Inspection[] = [
      makeInspection(toComparableNumber(data.config.minAnswer), toComparableNumber(input.minAnswer), 'Min Answer'),
      makeInspection(toComparableNumber(data.config.maxAnswer), toComparableNumber(input.maxAnswer), 'Max Answer'),
      makeInspection(toComparableNumber(data.config.decimals), toComparableNumber(input.decimals), 'Decimals'),
      makeInspection(
        // Description comes with some empty bytes
        Buffer.from(data.config.description.filter((v) => v !== 0)).toString(),
        input.description,
        'Description',
      ),
      makeInspection(
        toComparablePubKey(data.config.requesterAccessController),
        toComparablePubKey(input.requesterAccessController),
        'Requester access controller',
      ),
      makeInspection(
        toComparablePubKey(data.config.billingAccessController),
        toComparablePubKey(input.billingAccessController),
        'Requester access controller',
      ),
      makeInspection(
        data.oracles.xs.slice(0, data.oracles.len).map(({ transmitter }) => toComparablePubKey(transmitter)),
        input.transmitters.map(toComparablePubKey),
        'Transmitters',
      ),
    ]

    const isSuccessfulInspection = inspect(inspections)

    return {
      responses: [
        {
          tx: this.wrapInspectResponse(isSuccessfulInspection, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
