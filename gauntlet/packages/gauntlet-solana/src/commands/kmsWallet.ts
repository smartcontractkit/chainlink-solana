import { PublicKey, Transaction, VersionedTransaction, Keypair } from '@solana/web3.js'
import { KMSClient, GetPublicKeyCommand, SignCommand } from '@aws-sdk/client-kms'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaWallet, WalletTypes } from './wallet'

const isVersionedTransaction = (tx: Transaction | VersionedTransaction): tx is VersionedTransaction => {
  return 'version' in tx
}

// Ed25519 DER-encoded SubjectPublicKeyInfo (RFC 8410) has a fixed 12-byte header
// followed by the raw 32-byte public key.
const DER_ED25519_PREFIX_LENGTH = 12
const ED25519_PUBKEY_LENGTH = 32

export class KMSWallet extends SolanaWallet {
  publicKey: PublicKey
  private client: KMSClient
  private keyId: string

  private constructor(client: KMSClient, keyId: string, publicKey: PublicKey) {
    super()
    this.client = client
    this.keyId = keyId
    this.publicKey = publicKey
  }

  static create = async (keyId: string, keyRegion: string): Promise<KMSWallet> => {
    const client = new KMSClient({ region: keyRegion })

    const response = await client.send(new GetPublicKeyCommand({ KeyId: keyId }))
    if (!response.PublicKey) {
      throw new Error('Failed to retrieve public key from KMS')
    }

    const rawKey = new Uint8Array(response.PublicKey).slice(
      DER_ED25519_PREFIX_LENGTH,
      DER_ED25519_PREFIX_LENGTH + ED25519_PUBKEY_LENGTH,
    )
    if (rawKey.length !== ED25519_PUBKEY_LENGTH) {
      throw new Error(
        `Expected ${ED25519_PUBKEY_LENGTH}-byte Ed25519 public key, got ${rawKey.length} bytes. ` +
          'Ensure the KMS key uses the ECC_EDWARDS_ED25519 key spec.',
      )
    }

    const publicKey = new PublicKey(rawKey)
    logger.info(`KMS: Using ${publicKey.toString()}, keyId: ${keyId}`)
    return new KMSWallet(client, keyId, publicKey)
  }

  signTransaction = async <T extends Transaction | VersionedTransaction>(tx: T): Promise<T> => {
    logger.info('KMS: Request to sign transaction')

    let msg: Buffer
    if (isVersionedTransaction(tx)) {
      msg = Buffer.from(tx.message.serialize())
    } else {
      msg = tx.serializeMessage()
    }

    const response = await this.client.send(
      new SignCommand({
        KeyId: this.keyId,
        Message: msg,
        MessageType: 'RAW',
        SigningAlgorithm: 'ED25519_SHA_512' as any,
      }),
    )

    if (!response.Signature) {
      throw new Error('KMS signing failed: no signature returned')
    }

    tx.addSignature(this.publicKey, Buffer.from(response.Signature))
    return tx
  }

  signAllTransactions = async <T extends Transaction | VersionedTransaction>(txs: T[]): Promise<T[]> => {
    logger.warn('Signing multiple transactions with KMS')
    const signed: T[] = []
    for (const tx of txs) {
      signed.push(await this.signTransaction(tx))
    }
    return signed
  }

  get payer(): Keypair {
    throw new Error('Payer method not available on KMS wallet')
  }

  type = () => WalletTypes.KMS
}
