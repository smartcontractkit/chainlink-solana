import { Wallet } from '@coral-xyz/anchor'
import { Keypair, PublicKey, Transaction, VersionedTransaction } from '@solana/web3.js'
import TransportNodeHid from '@ledgerhq/hw-transport-node-hid'
import Solana from '@ledgerhq/hw-app-solana'
import { logger } from '@chainlink/gauntlet-core/dist/utils'

export enum WalletTypes {
  LOCAL = 'local',
  LEDGER = 'ledger',
}

export abstract class SolanaWallet {
  static create: (...args) => Promise<SolanaWallet>

  abstract signTransaction<T extends Transaction | VersionedTransaction>(tx: T): Promise<T>
  abstract signAllTransactions<T extends Transaction | VersionedTransaction>(txs: T[]): Promise<T[]>
  abstract publicKey: PublicKey
  abstract payer: Keypair

  abstract type: () => WalletTypes
}

export class LocalWallet extends SolanaWallet {
  wallet: Wallet

  private constructor(wallet: Wallet) {
    super()
    this.wallet = wallet
  }

  static create = async (keypair: Keypair) => {
    const wallet = new Wallet(keypair)
    return new LocalWallet(wallet)
  }

  async signTransaction<T extends Transaction | VersionedTransaction>(tx: T): Promise<T> {
    return this.wallet.signTransaction(tx)
  }
  async signAllTransactions<T extends Transaction | VersionedTransaction>(txs: T[]): Promise<T[]> {
    return this.wallet.signAllTransactions(txs)
  }

  get publicKey() {
    return this.wallet.payer.publicKey
  }

  get payer() {
    return this.wallet.payer
  }

  type = () => WalletTypes.LOCAL
}

const isVersionedTransaction = (tx: Transaction | VersionedTransaction): tx is VersionedTransaction => {
  return 'version' in tx
}

export class LedgerWallet extends SolanaWallet {
  publicKey: PublicKey
  wallet: Solana
  path: string

  private constructor(solanaLW: Solana, pubKey: PublicKey, path: string) {
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
      logger.info(`Ledger: Using ${pubkey.toString()}, derivationPath: ${path} (can be overridden with --ledgerPath)`)
      return new LedgerWallet(solana, pubkey, path)
    } catch (e) {
      logger.error('Ledger: Could not access ledger. Is it unlocked and Solana app open?')
      throw e
    }
  }

  signTransaction = async <T extends Transaction | VersionedTransaction>(tx: T) => {
    logger.info(`Ledger: Request to sign message`)
    // serializeMessage on v0, serialize on v1

    let msg: Buffer

    if (isVersionedTransaction(tx)) {
      msg = Buffer.from(tx.serialize())
    } else {
      msg = tx.serializeMessage()
    }
    const { signature } = await this.wallet.signTransaction(this.path, msg)
    tx.addSignature(this.publicKey, signature)
    return tx
  }

  signAllTransactions = async <T extends Transaction | VersionedTransaction>(txs: T[]) => {
    logger.warn('Signing multiple transactions with Ledger')
    return Promise.all(txs.map(this.signTransaction))
  }

  get payer(): Keypair {
    throw new Error('Payer method not available on Ledger')
  }

  type = () => WalletTypes.LEDGER
}
