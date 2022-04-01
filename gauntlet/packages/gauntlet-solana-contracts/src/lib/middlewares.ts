import { SolanaCommand, AddressBook } from '@chainlink/gauntlet-solana'
import logger from '../logger'
import { Middleware, Next } from '@chainlink/gauntlet-core'
import { CONTRACT_LIST } from './contracts'
import { validateAddress } from '@chainlink/gauntlet-solana/dist/lib/utils'

const addressBooks = new Map<string, AddressBook>()

// Loads known addresses for deployed contracts from environment & wallet
//
// Commands on the same network share the same addressBook
// The logger also needs a reference to addressBook for logger.styleAddress(),
// but currently supports only one network
export const withAddressBook: Middleware = (c: SolanaCommand, next: Next) => {
  const network = c.flags.network

  if (!addressBooks.has(network)) {
    addressBooks[network] = new AddressBook()
    addressBooks[network].setOperator(c.wallet.publicKey.toString())

    const tryAddInstance = (id: CONTRACT_LIST, address: string | undefined, name?: string) => {
      if (!address) {
        logger.warn(`Address for ${id} not set in environment`)
      } else if (!validateAddress(address)) {
        throw new Error(`Read invalid contract address ${address} for ${id} contract from env`)
      } else {
        addressBooks[network].addInstance(id, address, name)
      }
    }

    // Addresses of deployed instances read from env vars
    tryAddInstance(CONTRACT_LIST.TOKEN, process.env['LINK'], 'link')
    tryAddInstance(CONTRACT_LIST.ACCESS_CONTROLLER, process.env['BILLING_ACCESS_CONTROLLER'], 'billing_access')
    tryAddInstance(CONTRACT_LIST.ACCESS_CONTROLLER, process.env['REQUESTER_ACCESS_CONTROLLER'], 'requester_access')
    tryAddInstance(CONTRACT_LIST.ACCESS_CONTROLLER, process.env['LOWERING_ACCESS_CONTROLLER'], 'lowering_access')
    tryAddInstance(CONTRACT_LIST.MULTISIG, process.env['MULTISIG_ADDRESS'], 'multisig')

    // TODO: extend logger for multi-chain
    logger.withAddressBook(addressBooks[network])
  }

  c.addressBook = addressBooks[network]

  return next()
}
