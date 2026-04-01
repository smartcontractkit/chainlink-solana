import { PublicKey, Transaction, VersionedTransaction, VersionedMessage, Keypair } from '@solana/web3.js'
import { KMSClient, GetPublicKeyCommand, SignCommand } from '@aws-sdk/client-kms'
import { KMSWallet } from './kmsWallet'
import { WalletTypes } from './wallet'

// Mock AWS SDK
jest.mock('@aws-sdk/client-kms', () => {
  const mockSend = jest.fn()
  return {
    KMSClient: jest.fn().mockImplementation(() => ({ send: mockSend })),
    GetPublicKeyCommand: jest.fn(),
    SignCommand: jest.fn(),
    SigningAlgorithmSpec: { ED25519_SHA_512: 'ED25519_SHA_512' },
    __mockSend: mockSend,
  }
})

// Mock logger
jest.mock('@chainlink/gauntlet-core/dist/utils', () => ({
  logger: {
    info: jest.fn(),
    warn: jest.fn(),
    error: jest.fn(),
  },
}))

const { __mockSend: mockSend } = jest.requireMock('@aws-sdk/client-kms')

// DER-encoded Ed25519 SubjectPublicKeyInfo: 12-byte header + 32-byte raw key
const makeValidDerKey = (rawKey: Uint8Array): Uint8Array => {
  const derHeader = new Uint8Array([0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00])
  const full = new Uint8Array(44)
  full.set(derHeader, 0)
  full.set(rawKey, 12)
  return full
}

describe('KMSWallet', () => {
  const testKeyId = 'arn:aws:kms:us-east-1:123456789:key/test-key'
  const testRegion = 'us-east-1'
  const rawPubKey = Keypair.generate().publicKey.toBytes()
  const validDerKey = makeValidDerKey(rawPubKey)

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('create', () => {
    it('creates wallet with valid Ed25519 KMS key', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: validDerKey })

      const wallet = await KMSWallet.create(testKeyId, testRegion)

      expect(wallet.publicKey).toEqual(new PublicKey(rawPubKey))
      expect(wallet.type()).toBe(WalletTypes.KMS)
      expect(KMSClient).toHaveBeenCalledWith({ region: testRegion })
    })

    it('throws when GetPublicKey returns no key', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: undefined })

      await expect(KMSWallet.create(testKeyId, testRegion)).rejects.toThrow('Failed to retrieve public key from KMS')
    })

    it('throws when key is wrong size', async () => {
      // Only 20 bytes total — too short for 12 header + 32 key
      mockSend.mockResolvedValueOnce({ PublicKey: new Uint8Array(20) })

      await expect(KMSWallet.create(testKeyId, testRegion)).rejects.toThrow('Expected 32-byte Ed25519 public key')
    })
  })

  describe('signTransaction', () => {
    let wallet: KMSWallet

    beforeEach(async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: validDerKey })
      wallet = await KMSWallet.create(testKeyId, testRegion)
    })

    it('signs a legacy transaction', async () => {
      const tx = new Transaction()
      tx.recentBlockhash = 'GHtXQBsoZHVnNFa9YevAzFr17DJjgHXk3ycTKD5xD3Zi'
      tx.feePayer = wallet.publicKey

      const fakeSignature = new Uint8Array(64).fill(1)
      mockSend.mockResolvedValueOnce({ Signature: fakeSignature })

      const signed = await wallet.signTransaction(tx)
      expect(signed.signatures).toHaveLength(1)
      expect(SignCommand).toHaveBeenCalledWith(
        expect.objectContaining({
          KeyId: testKeyId,
          MessageType: 'RAW',
          SigningAlgorithm: 'ED25519_SHA_512',
        }),
      )
    })

    it('throws when KMS returns no signature', async () => {
      const tx = new Transaction()
      tx.recentBlockhash = 'GHtXQBsoZHVnNFa9YevAzFr17DJjgHXk3ycTKD5xD3Zi'
      tx.feePayer = wallet.publicKey

      mockSend.mockResolvedValueOnce({ Signature: undefined })

      await expect(wallet.signTransaction(tx)).rejects.toThrow('KMS signing failed: no signature returned')
    })
  })

  describe('signAllTransactions', () => {
    it('signs multiple transactions sequentially', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: validDerKey })
      const wallet = await KMSWallet.create(testKeyId, testRegion)

      const fakeSignature = new Uint8Array(64).fill(1)
      mockSend.mockResolvedValue({ Signature: fakeSignature })

      const tx1 = new Transaction()
      tx1.recentBlockhash = 'GHtXQBsoZHVnNFa9YevAzFr17DJjgHXk3ycTKD5xD3Zi'
      tx1.feePayer = wallet.publicKey

      const tx2 = new Transaction()
      tx2.recentBlockhash = 'GHtXQBsoZHVnNFa9YevAzFr17DJjgHXk3ycTKD5xD3Zi'
      tx2.feePayer = wallet.publicKey

      const signed = await wallet.signAllTransactions([tx1, tx2])
      expect(signed).toHaveLength(2)
    })
  })

  describe('payer', () => {
    it('throws when accessing payer', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: validDerKey })
      const wallet = await KMSWallet.create(testKeyId, testRegion)

      expect(() => wallet.payer).toThrow('Payer method not available on KMS wallet')
    })
  })
})
