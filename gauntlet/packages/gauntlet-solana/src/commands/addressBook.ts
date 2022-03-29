import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { assertions } from '@chainlink/gauntlet-core/dist/utils'

type ContractId = string

type Instance = {
  name: string
  contractId: ContractId
}

export class AddressBook {
  operator: string
  instances: Map<string, Instance> // address => instance name

  constructor() {
    this.instances = new Map<string, Instance>()
  }

  setOperator(address: string) {
    this.operator = address
  }
  addInstance(contractId: ContractId, address: string, name?: string) {
    this.instances.set(address, { name, contractId } as Instance)
    logger.debug(`Using deployed instance of ${contractId}: ${name}=${address}`)
    return this
  }
}
