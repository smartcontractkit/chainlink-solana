import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { createAssociatedTokenAccount, createMint, mintTo } from '@solana/spl-token'
import { TOKEN_DECIMALS } from '../../../lib/constants'
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

    const decimals = this.flags.decimals || TOKEN_DECIMALS
    const token = await createMint(
      this.provider.connection,
      this.wallet.payer,
      mintAuthority.publicKey,
      this.wallet.payer.publicKey, // Freeze authority
      decimals,
    )

    const billion = BigInt(Math.pow(10, 9))
    const tokenVault = await createAssociatedTokenAccount(this.provider.connection, this.wallet.payer, token, this.wallet.payer.publicKey)
    const mintAmount = billion * BigInt(Math.pow(10, decimals))

    await prompt(
      `Minting ${billion.toString()} token units, with ${decimals} decimals. Total ${mintAmount.toString()}. Continue?`,
    )

    await mintTo(this.provider.connection, this.wallet.payer, token, tokenVault, mintAuthority, mintAmount)

    // To disable minting https://github.com/solana-labs/solana-program-library/blob/36e886392b8c6619b275f6681aed6d8aae6e70f9/token/js/client/token.js#L985

    logger.info(`
      TOKEN:
        - Address: ${token}
      VAULT:
        - address: ${tokenVault.toString()}
      STATE ACCOUNTS:
        - Mint Authority: ${mintAuthority.publicKey}
        - Freeze Authority: ${this.wallet.payer.publicKey}
    `)

    return {
      responses: [
        {
          tx: { ...this.wrapResponse('', token.toString()), wait: async () => ({ success: true }) },
          contract: token.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
