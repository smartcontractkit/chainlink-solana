import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { createAssociatedTokenAccountInstruction, getAssociatedTokenAddress } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { isValidTokenAccount } from './utils'

export default class CreateAccount extends SolanaCommand {
  static id = 'token:create_account'
  static category = CONTRACT_LIST.TOKEN
  static examples = ['yarn gauntlet token:create_account --network=devnet --link=<TOKEN> [BASE_ADDRESS...]']

  constructor(flags, args) {
    super(flags, args)

    this.require(!!args[0], `Provide an address from which the 'Token Associated Account' will be derived`)
  }

  execute = async () => {
    // validate LINK address present
    this.require(this.flags.link || process.env.LINK, 'LINK token not found')

    const tokenAddress = new PublicKey(this.flags.link || process.env.LINK)

    const addresses = this.args.map((addr) => new PublicKey(addr))

    const ixs = await Promise.all(
      addresses.map(async (address) => {
        const associatedAcc = await getAssociatedTokenAddress(tokenAddress, address, true)

        const accountExists = await isValidTokenAccount(this.provider.connection, tokenAddress, associatedAcc)
        this.require(
          !accountExists,
          `A Token Associated Account to address ${address.toString()} already exists: ${associatedAcc}`,
        )

        return createAssociatedTokenAccountInstruction(this.wallet.publicKey, associatedAcc, address, tokenAddress)
      }),
    )

    await prompt(`Continue to create new Token associated accounts?`)
    logger.loading('Creating accounts...')
    const tx = await this.signAndSendRawTx(ixs)

    logger.success(`New accounts created on tx ${tx}`)
    // logger.success(`New account created at ${associatedAcc.toString()} on tx ${tx}`)

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
