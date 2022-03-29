import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { AddressBook } from './addressBook'
import { assertions } from '@chainlink/gauntlet-core/dist/utils'

type COLOR = 'red' | 'blue' | 'yellow' | 'green'
type INTENSITY = 'dim' | 'bright'
type Style = COLOR | INTENSITY
type Styles = { [id: string]: [color: COLOR, intensity: INTENSITY] }
const styles = {
  MULTISIG_LABEL: ['yellow', 'bright'],
  MULTISIG_ADDRESS: ['yellow', 'dim'],
  CONTRACT_LABEL: ['blue', 'bright'],
  CONTRACT_ADDRESS: ['blue', 'dim'],
  OPERATOR_LABEL: ['green', 'bright'],
  OPERATOR_ADDRESS: ['green', 'dim'],
} as Styles

// Shows up in terminal as single emoji (astronaut), but two emojis (adult + rocket) in some editors.
// TODO: check portability, possibly just use adult emoji?
//  https://emojiterra.com/astronaut-medium-skin-tone/

const formatMultisig = (address: string, label: string): string =>
  `🧳 ${logger.style(label, ...styles.MULTISIG_LABEL)}:${logger.style(address, ...styles.MULTISIG_ADDRESS)}`

const formatContract = (address: string, label: string): string =>
  `📜 ${logger.style(label, ...styles.CONTRACT_LABEL)}:${logger.style(address, ...styles.CONTRACT_ADDRESS)}`

const formatOperator = (address: string): string =>
  `🧑🏽 ${logger.style('operator', ...styles.OPERATOR_LABEL)}:${logger.style(address, ...styles.OPERATOR_ADDRESS)}`

export class SolanaLogger {
  addressBook: AddressBook

  withAddressBook(addressBook: AddressBook) {
    this.addressBook = addressBook
  }

  // Example:  logger.info(`Destination address is ${logger.styleAddress(address)}`)
  styleAddress(address: string): string {
    if (!this.addressBook) {
      logger.warn(`SolanaLogger: styleAddress called before calling withAddressBook!`)
      return address
    }

    if (this.addressBook.instances.has(address)) {
      const name = this.addressBook.instances.get(address).name
      if (name == 'multisig') {
        return formatMultisig(address, name)
      } else {
        return formatContract(address, name)
      }
    } else if (address == this.addressBook.operator) {
      return formatOperator(address)
    } else {
      return address
    }
  }
}
