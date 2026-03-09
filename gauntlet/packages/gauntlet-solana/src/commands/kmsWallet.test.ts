import { KMSWallet } from './kmsWallet'
import { KMSClient, GetPublicKeyCommand, SignCommand } from '@aws-sdk/client-kms'
import { PublicKey, Transaction, VersionedTransaction, TransactionMessage } from '@solana/web3.js'

// Mock AWS KMS client
jest.mock('@aws-sdk/client-kms', () => ({
  KMSClient: jest.fn(),
  GetPublicKeyCommand: jest.fn(),
  SignCommand: jest.fn(),
}))

// Mock logger to avoid console output during tests
jest.mock('@chainlink/gauntlet-core/dist/utils', () => ({
  logger: {
    info: jest.fn(),
    warn: jest.fn(),
    error: jest.fn(),
    loading: jest.fn(),
    success: jest.fn(),
    debug: jest.fn(),
    log: jest.fn(),
    line: jest.fn(),
  },
}))

// Ed25519 DER SubjectPublicKeyInfo: 12-byte header + 32-byte key
const DER_ED25519_HEADER = new Uint8Array(12)
const MOCK_ED25519_PUBKEY = new Uint8Array(32).fill(1)
const MOCK_PUBKEY_DER = new Uint8Array([...DER_ED25519_HEADER, ...MOCK_ED25519_PUBKEY])
const MOCK_SIGNATURE = new Uint8Array(64).fill(2)

describe('KMSWallet', () => {
  let mockSend: jest.Mock

  beforeEach(() => {
    jest.clearAllMocks()
    mockSend = jest.fn()
    ;(KMSClient as unknown as jest.Mock).mockImplementation(() => ({
      send: mockSend,
    }))
  })

  describe('create', () => {
    it('creates wallet with valid KMS public key response', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })

      const wallet = await KMSWallet.create('key-123', 'us-east-1')

      expect(wallet).toBeInstanceOf(KMSWallet)
      expect(wallet.publicKey).toBeInstanceOf(PublicKey)
      expect(wallet.publicKey.toBytes()).toEqual(MOCK_ED25519_PUBKEY)
      expect(mockSend).toHaveBeenCalledWith(expect.any(GetPublicKeyCommand))
    })

    it('throws when PublicKey is missing from KMS response', async () => {
      mockSend.mockResolvedValueOnce({})

      await expect(KMSWallet.create('key-123', 'us-east-1')).rejects.toThrow(
        'Failed to retrieve public key from KMS',
      )
    })

    it('throws when public key has wrong length', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: new Uint8Array(20) })

      await expect(KMSWallet.create('key-123', 'us-east-1')).rejects.toThrow(
        'Expected 32-byte Ed25519 public key',
      )
    })
  })

  describe('signTransaction', () => {
    it('signs a legacy transaction', async () => {
      mockSend
        .mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })
        .mockResolvedValueOnce({ Signature: MOCK_SIGNATURE })

      const wallet = await KMSWallet.create('key-123', 'us-east-1')
      const tx = new Transaction()
      tx.feePayer = wallet.publicKey
      tx.recentBlockhash = '11111111111111111111111111111111'

      const signed = await wallet.signTransaction(tx)

      expect(signed.signatures).toHaveLength(1)
      expect(signed.signatures[0].signature).toEqual(MOCK_SIGNATURE)
      expect(mockSend).toHaveBeenCalledTimes(2)
    })

    it('signs a versioned transaction', async () => {
      mockSend
        .mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })
        .mockResolvedValueOnce({ Signature: MOCK_SIGNATURE })

      const wallet = await KMSWallet.create('key-123', 'us-east-1')
      const message = new TransactionMessage({
        payerKey: new PublicKey(MOCK_ED25519_PUBKEY),
        recentBlockhash: '11111111111111111111111111111111',
        instructions: [],
      }).compileToLegacyMessage()
      const tx = new VersionedTransaction(message)

      const signed = await wallet.signTransaction(tx)

      expect(signed.signatures).toHaveLength(1)
      expect(mockSend).toHaveBeenCalledTimes(2)
    })

    it('throws when KMS returns no signature', async () => {
      mockSend
        .mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })
        .mockResolvedValueOnce({})

      const wallet = await KMSWallet.create('key-123', 'us-east-1')
      const tx = new Transaction()
      tx.feePayer = wallet.publicKey
      tx.recentBlockhash = '11111111111111111111111111111111'

      await expect(wallet.signTransaction(tx)).rejects.toThrow(
        'KMS signing failed: no signature returned',
      )
    })
  })

  describe('signAllTransactions', () => {
    it('signs multiple transactions', async () => {
      mockSend
        .mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })
        .mockResolvedValueOnce({ Signature: MOCK_SIGNATURE })
        .mockResolvedValueOnce({ Signature: MOCK_SIGNATURE })

      const wallet = await KMSWallet.create('key-123', 'us-east-1')
      const tx1 = new Transaction()
      tx1.feePayer = wallet.publicKey
      tx1.recentBlockhash = '11111111111111111111111111111111'
      const tx2 = new Transaction()
      tx2.feePayer = wallet.publicKey
      tx2.recentBlockhash = '11111111111111111111111111111111'

      const signed = await wallet.signAllTransactions([tx1, tx2])

      expect(signed).toHaveLength(2)
      expect(mockSend).toHaveBeenCalledTimes(3)
    })
  })

  describe('payer', () => {
    it('throws when payer is accessed', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })

      const wallet = await KMSWallet.create('key-123', 'us-east-1')

      expect(() => wallet.payer).toThrow('Payer method not available on KMS wallet')
    })
  })

  describe('type', () => {
    it('returns KMS wallet type', async () => {
      mockSend.mockResolvedValueOnce({ PublicKey: MOCK_PUBKEY_DER })

      const wallet = await KMSWallet.create('key-123', 'us-east-1')

      expect(wallet.type()).toBe('kms')
    })
  })
})
