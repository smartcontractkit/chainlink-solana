import { Middleware, Next } from '@chainlink/gauntlet-core'
import { boolean } from '@chainlink/gauntlet-core/dist/lib/args'
import { assertions, logger } from '@chainlink/gauntlet-core/dist/utils'
import { AnchorProvider } from '@project-serum/anchor'
import { Connection, Keypair } from '@solana/web3.js'
import { DEFAULT_DERIVATION_PATH } from '../lib/constants'
import SolanaCommand, { CONFIRM_LEVEL } from './internal/solana'
import { LedgerWallet, LocalWallet } from './wallet'

const isValidURL = (url: string) => {
  var pattern = new RegExp('^(https?)://')
  return pattern.test(url)
}
export const withProvider: Middleware = (c: SolanaCommand, next: Next) => {
  const nodeURL = process.env.NODE_URL
  assertions.assert(
    nodeURL && isValidURL(nodeURL),
    `Invalid NODE_URL (${nodeURL}), please add an http:// or https:// prefix`,
  )

  c.provider = new AnchorProvider(new Connection(nodeURL), c.wallet, {
    preflightCommitment: CONFIRM_LEVEL,
    commitment: CONFIRM_LEVEL,
  })
  return next()
}

export const withWallet: Middleware = async (c: SolanaCommand, next: Next) => {
  if (c.flags.withLedger || boolean(process.env.WITH_LEDGER)) {
    logger.info('Loading Ledger wallet')
    const path = c.flags.ledgerPath || DEFAULT_DERIVATION_PATH
    c.wallet = await LedgerWallet.create(path)
    console.info(`Operator address is ${c.wallet.publicKey}`)
    return next()
  }

  logger.info('Loading Local wallet')
  const rawPK = process.env.PRIVATE_KEY
  assertions.assert(!!rawPK, `Missing PRIVATE_KEY, please add one`)

  const keypair = Keypair.fromSecretKey(Uint8Array.from(JSON.parse(rawPK)))
  c.wallet = await LocalWallet.create(keypair)
  console.info(`Operator address is ${c.wallet.publicKey}`)
  return next()
}

export const withNetwork: Middleware = (c: SolanaCommand, next: Next) => {
  assertions.assert(
    !!c.flags.network,
    `Network required. Invalid network (${c.flags.network}), please specify a --network`,
  )
  return next()
}
