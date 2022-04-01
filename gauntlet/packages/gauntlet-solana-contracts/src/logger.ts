import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaLogger, AddressBook } from '@chainlink/gauntlet-solana'

const solanaLogger = new SolanaLogger()

export default {
  styleAddress: (address: string) => solanaLogger.styleAddress(address),
  withAddressBook: (addressBook: AddressBook) => solanaLogger.withAddressBook(addressBook),
  ...logger,
}
