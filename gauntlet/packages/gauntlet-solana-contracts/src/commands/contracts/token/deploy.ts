import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import {
  createInitializeMintInstruction,
  createAssociatedTokenAccountInstruction,
  createMintToInstruction,
  getAssociatedTokenAddress,
  MINT_SIZE,
  TOKEN_PROGRAM_ID,
} from '@solana/spl-token'
import { Keypair, SystemProgram } from '@solana/web3.js'
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
    const mintAuthority = this.wallet.publicKey

    logger.loading('Creating token...')

    const decimals = this.flags.decimals || TOKEN_DECIMALS
    const mint = Keypair.generate()
    const lamports = await this.provider.connection.getMinimumBalanceForRentExemption(MINT_SIZE)

    const createMintIxs = [
      SystemProgram.createAccount({
        fromPubkey: this.wallet.publicKey,
        newAccountPubkey: mint.publicKey,
        space: MINT_SIZE,
        lamports,
        programId: TOKEN_PROGRAM_ID,
      }),
      createInitializeMintInstruction(mint.publicKey, decimals, mintAuthority, mintAuthority),
    ]

    const tokenVault = await getAssociatedTokenAddress(mint.publicKey, this.wallet.publicKey)
    const createAtaIx = createAssociatedTokenAccountInstruction(
      this.wallet.publicKey,
      tokenVault,
      this.wallet.publicKey,
      mint.publicKey,
    )

    await this.signAndSendRawTx([...createMintIxs, createAtaIx], [mint])

    const billion = BigInt(Math.pow(10, 9))
    const mintAmount = billion * BigInt(Math.pow(10, decimals))

    await prompt(
      `Minting ${billion.toString()} token units, with ${decimals} decimals. Total ${mintAmount.toString()}. Continue?`,
    )

    const mintToIx = createMintToInstruction(mint.publicKey, tokenVault, mintAuthority, mintAmount)
    await this.signAndSendRawTx([mintToIx])

    logger.info(`
      TOKEN:
        - Address: ${mint.publicKey}
      VAULT:
        - address: ${tokenVault.toString()}
      STATE ACCOUNTS:
        - Mint Authority: ${mintAuthority}
        - Freeze Authority: ${mintAuthority}
    `)

    return {
      data: {
        vault: tokenVault.toString(),
      },
      responses: [
        {
          tx: { ...this.wrapResponse('', mint.publicKey.toString()), wait: async () => ({ success: true }) },
          contract: mint.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
