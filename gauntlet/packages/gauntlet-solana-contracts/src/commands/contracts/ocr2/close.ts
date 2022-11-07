import { getAssociatedTokenAddress, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { utils } from '@project-serum/anchor'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import Close from '../../abstract/close'
import { logger } from '@chainlink/gauntlet-core/dist/utils'

export default class extends Close {
  static id = Close.makeId(CONTRACT_LIST.OCR_2)
  static category = Close.makeCategory(CONTRACT_LIST.OCR_2)
  static examples = Close.makeExamples(CONTRACT_LIST.OCR_2)
  static description = Close.makeDescription(CONTRACT_LIST.OCR_2)

  constructor(flags, args) {
    super(flags, args)

    this.contractId = CONTRACT_LIST.OCR_2
  }

  makeRawTransaction = async (signer) => {
    const contract = getContract(this.contractId, '')
    const program = this.loadProgram(contract.idl, contract.programId.toString())

    const address = new PublicKey(this.args[0])
    const linkPublicKey = new PublicKey(this.flags.link || process.env.LINK)
    const tokenReceiver = await getAssociatedTokenAddress(linkPublicKey, signer, true)
    logger.info(`This command involves a token payout. The receiver will be: ${tokenReceiver}`)

    const { config, oracles } = (await program.account.state.fetch(address)) as any
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), address.toBuffer()],
      program.programId,
    )
    // get payees to payout during close command
    const payees = oracles.xs
      .slice(0, oracles.len)
      .map((oracle) => ({ pubkey: oracle.payee, isWritable: true, isSigner: false }))

    const extraAccounts = {
      tokenReceiver,
      tokenVault: config.tokenVault,
      vaultAuthority,
      tokenProgram: TOKEN_PROGRAM_ID,
      state: address,
    }

    return this.prepareInstructions(signer, extraAccounts, 'close', payees)
  }
}
