import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { utils } from '@project-serum/anchor'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class PayRemaining extends SolanaCommand {
  static id = 'ocr2:pay_remaining'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:pay_remaining --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const owner = this.wallet.payer

    const data = await program.account.state.fetch(state)
    console.log(data)
    const billingAC = new PublicKey(data.config.billingAccessController)
    const link = new PublicKey(this.flags.link)

    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )

    const tokenVault = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      link,
      vaultAuthority,
      true,
    )

    // Get remaining non paid oracles. New oracles will be placed at the beginning of leftoverPayments
    const leftovers = data.leftoverPayments.slice(0, data.leftoverPaymentsLen)
    const remainingAccounts = leftovers.map((leftover) => ({
      pubkey: new PublicKey(leftover.payee),
      isWritable: true,
      isSigner: false,
    }))

    this.require(remainingAccounts.length > 0, 'No remaining oracles to pay')

    logger.log(
      'Paying remaining accounts:',
      remainingAccounts.map(({ pubkey }) => pubkey.toString()),
    )
    await prompt('Continue pay remaining oracles?')
    const tx = await program.rpc.payRemaining({
      accounts: {
        state,
        authority: owner.publicKey,
        accessController: billingAC,
        tokenVault,
        vaultAuthority,
        tokenProgram: TOKEN_PROGRAM_ID,
      },
      remainingAccounts,
      signers: [owner],
    })

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
