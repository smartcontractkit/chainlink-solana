import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
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

    const decimals = this.flags.decimals || 18
    const token = await Token.createMint(
      this.provider.connection,
      this.wallet.payer,
      mintAuthority.publicKey,
      null,
      decimals,
      TOKEN_PROGRAM_ID,
    )

    const tokenVault = await token.createAssociatedTokenAccount(this.wallet.publicKey)
    const mintAmount = new BN(['1'].concat(new Array(decimals).fill('0')).join(''))
    logger.log('Minting', mintAmount.toString())

    await token.mintTo(tokenVault, this.wallet.payer, [], mintAmount)

    logger.info(`
      TOKEN:
        - Address: ${token.publicKey}
      VAULT:
        - address: ${tokenVault.toString()}
      STATE ACCOUNTS:
        - Mint Authority: ${mintAuthority.publicKey}
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
