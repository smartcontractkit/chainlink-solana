import { Middleware, Next } from '@chainlink/gauntlet-core'
import { boolean } from '@chainlink/gauntlet-core/dist/lib/args'
import { assertions, logger } from '@chainlink/gauntlet-core/dist/utils'
import { AnchorProvider } from '@coral-xyz/anchor'
import { Connection, Keypair } from '@solana/web3.js'
import { DEFAULT_DERIVATION_PATH } from '../lib/constants'
import SolanaCommand from './internal/solana'
import { LedgerWallet, LocalWallet } from './wallet'

const isValidURL = (url: string) => {
  const pattern = new RegExp('^(https?|wss?):/')
  return pattern.test(url)
}
export const withProvider: Middleware = (c: SolanaCommand, next: Next) => {
  const nodeURL = process.env.NODE_URL
  assertions.assert(
    nodeURL && isValidURL(nodeURL),
    `Invalid NODE_URL (${nodeURL}), please add an http:// or https:// prefix`,
  )
  const wsUrl = process.env.WS_URL
  if (wsUrl) {
    assertions.assert(isValidURL(wsUrl), `Invalid WS_URL (${wsUrl}), please add an ws:// or wss:// prefix`)
  }
  c.provider = new AnchorProvider(new Connection(nodeURL, wsUrl ? { wsEndpoint: wsUrl } : {}), c.wallet, {
    preflightCommitment: 'confirmed', // use confirmed commitment
    commitment: 'confirmed',
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
