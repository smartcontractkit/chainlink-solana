import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class AddAccess extends SolanaCommand {
  static id = 'access_controller:add_access'
  static category = CONTRACT_LIST.ACCESS_CONTROLLER

  static examples = [
    'yarn gauntlet access_controller:add_access --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --address=9ohrpVDVNKKW1LipksFrmq6wa1oLLYL9QSoYUn4pAQ2v',
    'yarn gauntlet access_controller:add_access --network=devnet --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR --address=9ohrpVDVNKKW1LipksFrmq6wa1oLLYL9QSoYUn4pAQ2v',
  ]
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state && !!this.flags.address, 'Please provide flags with "state" and "address"')
  }

  execute = async () => {
    const accessController = getContract(CONTRACT_LIST.ACCESS_CONTROLLER, '')
    const address = accessController.programId.publicKey.toString()
    const program = this.loadProgram(accessController.idl, address)

    const state = new PublicKey(this.flags.state)
    const accessAddress = new PublicKey(this.flags.address)

    console.log(`Giving access to ${accessAddress}...`)
    const tx = await program.rpc.addAccess({
      accounts: {
        state: state,
        owner: this.wallet.payer.publicKey,
        address: accessAddress,
      },
      signers: [this.wallet.payer],
    })

    logger.success(`Access given on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, address, { state: state.toString() }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
