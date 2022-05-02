import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { AddressBook } from '@chainlink/gauntlet-core'
import { Logger } from '@chainlink/gauntlet-core/dist/utils/logger'

const solanaLogger = new Logger()

export default {
  styleAddress: (address: string) => solanaLogger.styleAddress(address),
  withAddressBook: (addressBook: AddressBook) => solanaLogger.withAddressBook(addressBook),
  ...logger,
}