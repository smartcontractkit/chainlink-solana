import { Result } from '@chainlink/gauntlet-core'
import { Proto } from '@chainlink/gauntlet-core/dist/crypto'
import { BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'

import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { descriptor as OCR2Descriptor } from '../../../lib/ocr2Proto'

export default class ReadState extends SolanaCommand {
  static id = 'ocr2:read_state'
  static category = CONTRACT_LIST.OCR_2

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state""')
  }

  deserializeConfig = async (buffer: Buffer): Promise<any> => {
    const proto = new Proto.Protobuf({ descriptor: OCR2Descriptor })
    const offchain = proto.decode('offchainreporting2_config.OffchainConfigProto', buffer)
    const reportingPluginConfig = proto.decode(
      'offchainreporting2_config.ReportingPluginConfig',
      offchain.reportingPluginConfig,
    )
    return { ...offchain, reportingPluginConfig }
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const state = new PublicKey(this.flags.state)
    // read could be abstract. account.accessController is just the name of the account that can be got form the camelcase(schema.accounts[x].name)
    const data = await program.account.state.fetch(state)
    const transmissions = new PublicKey(data.transmissions)
    const transmissionsData = await program.account.transmissions.fetch(transmissions)

    const validTransmissions = transmissionsData.transmissions
      .map(({ answer, timestamp }) => {
        const value = new BN(answer)
        if (!!value.toNumber()) {
          return { value: value.toString(), timestamp }
        }
      })
      .filter((v) => !!v)
    console.log('DATA:', data)
    // Get the necessary bytes
    const offchainBuffer = Buffer.from(data.config.offchainConfig.xs).slice(
      0,
      new BN(data.config.offchainConfig.len).toNumber(),
    )
    const offchainConfig = await this.deserializeConfig(offchainBuffer)
    console.log('GENERATED LENGTH:', offchainBuffer.byteLength)

    console.log('OFFCHAIN CONFIG:', offchainConfig)
    console.log('TRANSMISSIONS:', validTransmissions)
    return {} as Result<TransactionResponse>
  }
}
