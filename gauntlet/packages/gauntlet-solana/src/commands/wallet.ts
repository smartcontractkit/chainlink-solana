const { Wallet } = require('@project-serum/anchor')
import { Wallet as SerumWallet } from '@project-serum/anchor'
import { Keypair, PublicKey, Transaction } from '@solana/web3.js'
import TransportNodeHid from '@ledgerhq/hw-transport-node-hid'
import Solana from '@ledgerhq/hw-app-solana'
import { logger } from '@chainlink/gauntlet-core/dist/utils'

export enum WalletTypes {
  LOCAL = 'local',
  LEDGER = 'ledger',
}

export abstract class SolanaWallet {
  static create: (...args) => Promise<SolanaWallet>

  abstract signTransaction: (tx: Transaction) => Promise<Transaction>
  abstract signAllTransactions: (txs: Transaction[]) => Promise<Transaction[]>
  abstract publicKey: PublicKey
  abstract payer: Keypair

  abstract type: () => WalletTypes
}

export class LocalWallet extends SolanaWallet {
  wallet: typeof SerumWallet

  constructor(wallet: typeof SerumWallet) {
    super()
    this.wallet = wallet
  }

  static create = async (keypair: Keypair) => {
    const wallet = new Wallet(keypair)
    return new LocalWallet(wallet)
  }

  signTransaction = (tx: Transaction) => this.wallet.signTransaction(tx)
  signAllTransactions = (txs: Transaction[]) => this.wallet.signAllTransactions(txs)

  get publicKey() {
    return this.wallet.payer.publicKey
  }

  get payer() {
    return this.wallet.payer
  }

  type = () => WalletTypes.LOCAL
}

export class LedgerWallet extends SolanaWallet {
  publicKey: PublicKey
  wallet: Solana
  path: string

  constructor(solanaLW: Solana, pubKey: PublicKey, path: string) {
    super()

    this.wallet = solanaLW
    this.publicKey = pubKey
    this.path = path
  }

  static create = async (path: string) => {
    try {
      const transport = await TransportNodeHid.create()
      const solana = new Solana(transport)
      const { address } = await solana.getAddress(path, false)
      const pubkey = new PublicKey(address)
      logger.info(`Ledger: Using ${pubkey.toString()}`)
      return new LedgerWallet(solana, pubkey, path)
    } catch (e) {
      logger.error('Ledger: Could not access ledger. Is it unlocked and Solana app open?')
      throw e
    }
  }

  signTransaction = async (tx: Transaction) => {
    logger.info(`Ledger: Request to sign message`)
    const msg = tx.serializeMessage()
    const { signature } = await this.wallet.signTransaction(this.path, msg)
    tx.addSignature(this.publicKey, signature)
    return tx
  }

  signAllTransactions = async (txs: Transaction[]) => {
    logger.warn('Signing multiple transactions with Ledger')
    return Promise.all(txs.map(this.signTransaction))
  }

  get payer(): Keypair {
    throw new Error('Payer method not available on Ledger')
  }

  type = () => WalletTypes.LEDGER
}
