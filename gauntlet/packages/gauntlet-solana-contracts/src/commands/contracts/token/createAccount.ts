import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { ASSOCIATED_TOKEN_PROGRAM_ID, TOKEN_PROGRAM_ID, createAssociatedTokenAccountInstruction, getAssociatedTokenAddress } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { isValidTokenAccount } from './utils'

export default class CreateAccount extends SolanaCommand {
  static id = 'token:create_account'
  static category = CONTRACT_LIST.TOKEN
  static examples = ['yarn gauntlet token:create_account --network=devnet --address=<BASE_ADDRESS> <TOKEN>']

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('address', `Provide an address from which the 'Token Associated Account' will be derived`)
    this.require(!!args[0], 'Provide a token address')
  }

  execute = async () => {
    const tokenAddress = new PublicKey(this.args[0])

    const newAccountBase = new PublicKey(this.flags.address)
    const associatedAcc = await getAssociatedTokenAddress(
      tokenAddress,
      newAccountBase,
      true,
    )

    // const token = new Token(this.provider.connection, tokenAddress, TOKEN_PROGRAM_ID, {
    //   publicKey: this.wallet.publicKey,
    //   secretKey: Buffer.from([]),
    // })

    const accountExists = await isValidTokenAccount(token, associatedAcc)
    this.require(
      !accountExists,
      `A Token Associated Account to address ${newAccountBase.toString()} already exists: ${associatedAcc}`,
    )

    const ix = createAssociatedTokenAccountInstruction(
      this.wallet.publicKey,
      associatedAcc,
      newAccountBase,
      tokenAddress,
    )

    await prompt(`Continue to create new Token associated account to ${newAccountBase.toString()}`)
    logger.loading('Creating account...')
    const tx = await this.signAndSendRawTx([ix])

    logger.success(`New account created at ${associatedAcc.toString()} on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, this.args[0]),
          contract: this.args[0],
        },
      ],
    } as Result<TransactionResponse>
  }
}
