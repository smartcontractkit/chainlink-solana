import { TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import Close from '../../abstract/close'

export default class CloseFeed extends Close {
  static id = `${CONTRACT_LIST.STORE}:close_feed`
  static category = Close.makeCategory(CONTRACT_LIST.STORE)
  static examples = Close.makeExamples(CONTRACT_LIST.STORE)
  static description = Close.makeDescription(CONTRACT_LIST.STORE)

  constructor(flags, args) {
    super(flags, args)

    this.contractId = CONTRACT_LIST.STORE
  }

  makeRawTransaction = async (signer) => {
    const contract = getContract(this.contractId, '')
    const program = this.loadProgram(contract.idl, contract.programId.toString())

    const transmissions = new PublicKey(this.args[0])
    const info = (await program.account.transmissions.fetch(transmissions)) as any
    const extraAccounts = {
      feed: transmissions,
      owner: new PublicKey(info.owner),
      tokenProgram: TOKEN_PROGRAM_ID,
    }
    return this.prepareInstructions(signer, extraAccounts, 'closeFeed', [], 'feed')
  }
}
