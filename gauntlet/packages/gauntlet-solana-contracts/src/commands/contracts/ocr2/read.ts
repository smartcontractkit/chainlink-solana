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

    const state = new PublicKey(this.args[0])
    // read could be abstract. account.accessController is just the name of the account that can be got form the camelcase(schema.accounts[x].name)
    const data = (await program.account.state.fetch(state)) as any

    console.log('OWNER:', new PublicKey(data.config.owner).toString())
    console.log('DATA:', data)
    // Get the necessary bytes
    const offchainBuffer = Buffer.from(data.offchainConfig.xs).slice(0, new BN(data.offchainConfig.len).toNumber())
    const offchainConfig = await this.deserializeConfig(offchainBuffer)
    console.log('GENERATED LENGTH:', offchainBuffer.byteLength)

    console.log('OFFCHAIN CONFIG:', offchainConfig)
    return {} as Result<TransactionResponse>
  }
}
