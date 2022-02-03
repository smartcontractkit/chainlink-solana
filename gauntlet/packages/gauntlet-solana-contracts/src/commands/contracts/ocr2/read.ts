import { Result } from '@chainlink/gauntlet-core'
import { Proto } from '@chainlink/gauntlet-core/dist/crypto'
import { BN, logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'

import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { descriptor as OCR2Descriptor } from '../../../lib/ocr2Proto'

export default class ReadState extends SolanaCommand {
  static id = 'ocr2:read_state'
  static category = CONTRACT_LIST.OCR_2

  constructor(flags, args) {
    super(flags, args)

    // this.require(!!this.flags.state, 'Please provide flags with "state""')
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
    await this.inspectTransferOwnership()

    // const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    // const program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    // const state = new PublicKey(this.flags.state)
    // // read could be abstract. account.accessController is just the name of the account that can be got form the camelcase(schema.accounts[x].name)
    // const data = await program.account.state.fetch(state)

    // console.log('OWNER:', new PublicKey(data.config.owner).toString())
    // // console.log('DATA:', data)
    // // Get the necessary bytes
    // const offchainBuffer = Buffer.from(data.config.offchainConfig.xs).slice(
    //   0,
    //   new BN(data.config.offchainConfig.len).toNumber(),
    // )
    // const offchainConfig = await this.deserializeConfig(offchainBuffer)
    // // console.log('GENERATED LENGTH:', offchainBuffer.byteLength)

    // // console.log('OFFCHAIN CONFIG:', offchainConfig)
    return {} as Result<TransactionResponse>
  }

  inspectTransferOwnership = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const ocr2Program = this.loadProgram(ocr2.idl, ocr2.programId.toString())

    const ac = getContract(CONTRACT_LIST.ACCESS_CONTROLLER, '')
    const acProgram = this.loadProgram(ac.idl, ac.programId.toString())

    const feeds = [
      '57X5Rq3o7k5z976kAjYTWu5yKfgX1aQxH4bXACpmTPPF',
      '5XvR36Yybeh7wMYfR1i3YDu9pcrpgacDujA84nvuCHgD',
      '7ytfPg43gSnGwe5ahQEtjyFp5qRjqgCQV396YQHFtX6Y',
      '8earjafHwUdcsTdVH8zpuU6ozvcxzgYkSjfKS12aag9o',
      '9QjW5o4gQhj5JggLh49bmWwiDevXLQoe1SMYwDKGgfDf',
      'BXNGDywcpX2zKCKHocB2fMjFfgdrvp8aGY6jWWBZbeF5',
    ]

    const acs = [
      'B8Wy43nTJcrmFBXhwxNjoy5eWSEMGaz2JfKwLocXu4Kk',
      '12E73QpK2JgqHgHcrnfJKNhaAi4nTbUhaaH29Dc73JvP',
      'CkNsLFuaX79bPMaTsQD6vM8ZG4N1HfsybJgJ1Gh6Kr7w',
    ]

    const expectedOwner = new PublicKey('BMKk78WEmZCQPptNYET8jdwf4EnsXY4WS74TBWjiSAbb')

    const feedsData = await Promise.all(
      feeds.map(async (feed) => {
        const data = await ocr2Program.account.state.fetch(new PublicKey(feed))
        return {
          feed: new PublicKey(feed),
          owner: new PublicKey(data.config.owner),
          proposedOwner: new PublicKey(data.config.proposedOwner),
        }
      }),
    )

    const acsData = await Promise.all(
      acs.map(async (ac) => {
        const data = await acProgram.account.accessController.fetch(new PublicKey(ac))
        return {
          ac: new PublicKey(ac),
          owner: new PublicKey(data.owner),
          proposedOwner: new PublicKey(data.proposedOwner),
        }
      }),
    )

    feedsData.forEach(({ feed, owner, proposedOwner }) => {
      logger.info(`FEED: ${feed.toString()}`)
      const transferCompleted = expectedOwner.toString() === owner.toString()
      if (transferCompleted) {
        logger.success('Transfer completed')
      } else {
        logger.warn(`Transfer not completed. Current proposed owner: ${proposedOwner.toString()}`)
      }
      logger.line()
    })

    acsData.forEach(({ ac, owner, proposedOwner }) => {
      logger.info(`AC: ${ac.toString()}`)
      const transferCompleted = expectedOwner.toString() === owner.toString()
      if (transferCompleted) {
        logger.success('Transfer completed')
      } else {
        logger.warn(`Transfer not completed. Current proposed owner: ${proposedOwner.toString()}`)
      }
      logger.line()
    })
  }
}
