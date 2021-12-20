import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import BN from 'bn.js'
import { CONTRACT_LIST } from '../../../lib/contracts'

export default class DeployToken extends SolanaCommand {
  static id = 'token:deploy'
  static category = CONTRACT_LIST.TOKEN
  static examples = ['yarn gauntlet token:deploy --network=devnet']

  constructor(flags, args) {
    super(flags, args)
  }

  execute = async () => {
    const mintAuthority = this.wallet.payer

    logger.loading('Creating token...')

    const decimals = this.flags.decimals || 9
    const token = await Token.createMint(
      this.provider.connection,
      this.wallet.payer,
      mintAuthority.publicKey,
      this.wallet.payer.publicKey, // Freeze authority
      decimals,
      TOKEN_PROGRAM_ID,
    )

    const billion = new BN(Math.pow(10, 9))
    const tokenVault = await token.createAssociatedTokenAccount(this.wallet.payer.publicKey)
    const mintAmount = billion.mul(new BN(Math.pow(10, decimals)))

    await prompt(
      `Minting ${billion.toString()} token units, with ${decimals} decimals. Total ${mintAmount.toString()}. Continue?`,
    )

    await token.mintTo(tokenVault, this.wallet.payer, [], mintAmount)

    logger.info(`
      TOKEN:
        - Address: ${token.publicKey}
      VAULT:
        - address: ${tokenVault.toString()}
      STATE ACCOUNTS:
        - Mint Authority: ${mintAuthority.publicKey}
        - Freeze Authority: ${this.wallet.payer.publicKey}
    `)

    return {
      responses: [
        {
          tx: { ...this.wrapResponse('', token.publicKey.toString()), wait: async () => ({ success: true }) },
          contract: token.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
