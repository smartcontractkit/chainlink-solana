import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { RawTransaction, SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { utils } from '@project-serum/anchor'
import { ASSOCIATED_TOKEN_PROGRAM_ID, Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'
import { makeTx } from '../../../lib/utils'

export default class PayRemaining extends SolanaCommand {
  static id = 'ocr2:pay_remaining'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:pay_remaining --network=devnet EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.args[0])
    const stateData = await program.account.state.fetch(state)
    const billingAC = new PublicKey(stateData.config.billingAccessController)
    const link = new PublicKey(this.flags.link || process.env.LINK)

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
    const leftovers = stateData.leftoverPayments.xs.slice(0, new BN(stateData.leftoverPayments.len).toNumber())
    const remainingAccounts = leftovers.map((leftover) => ({
      pubkey: new PublicKey(leftover.payee),
      isWritable: true,
      isSigner: false,
    }))

    this.require(remainingAccounts.length > 0, 'No remaining oracles to pay')
    logger.log(
      'Remaining accounts to pay:',
      remainingAccounts.map(({ pubkey }) => pubkey.toString()),
    )

    const data = program.coder.instruction.encode('pay_remaining', {})

    const accounts: AccountMeta[] = [
      {
        pubkey: state,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: signer,
        isWritable: false,
        isSigner: true,
      },
      {
        pubkey: billingAC,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: tokenVault,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: vaultAuthority,
        isWritable: false,
        isSigner: false,
      },
      {
        pubkey: TOKEN_PROGRAM_ID,
        isWritable: false,
        isSigner: false,
      },
      ...remainingAccounts,
    ]

    const rawTx: RawTransaction = {
      data,
      accounts,
      programId: ocr2.programId,
    }

    return [rawTx]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Pay remaining on ${this.args[0].toString()}?`)
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Remaining oracles paid on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.args[0].toString()),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
