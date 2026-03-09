import { withWallet } from './middlewares'
import { KMSWallet } from './kmsWallet'
import { PublicKey } from '@solana/web3.js'

jest.mock('./kmsWallet', () => ({
  KMSWallet: {
    create: jest.fn(),
  },
}))

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
  assertions: {
    assert: jest.fn((condition: boolean, message: string) => {
      if (!condition) throw new Error(message)
    }),
  },
}))

const mockCreate = KMSWallet.create as jest.Mock

describe('withWallet', () => {
  const originalEnv = process.env
  let mockNext: jest.Mock
  let mockCommand: { flags: Record<string, unknown>; wallet?: unknown }

  beforeEach(() => {
    jest.clearAllMocks()
    process.env = { ...originalEnv }
    mockNext = jest.fn().mockResolvedValue(undefined)
    mockCommand = {
      flags: {},
    }
  })

  afterEach(() => {
    process.env = originalEnv
  })

  describe('KMS path', () => {
    it('creates KMS wallet when --withKms flag is set', async () => {
      process.env.KMS_KEY_ID = 'arn:aws:kms:us-east-1:123:key/abc'
      process.env.KMS_KEY_REGION = 'us-east-1'
      mockCommand.flags = { withKms: true }

      const mockWallet = { publicKey: new PublicKey(new Uint8Array(32).fill(1)) }
      mockCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as any, mockNext)

      expect(mockCreate).toHaveBeenCalledWith('arn:aws:kms:us-east-1:123:key/abc', 'us-east-1')
      expect(mockCommand.wallet).toBe(mockWallet)
      expect(mockNext).toHaveBeenCalled()
    })

    it('creates KMS wallet when WITH_KMS env is set', async () => {
      process.env.WITH_KMS = 'true'
      process.env.KMS_KEY_ID = 'key-123'
      process.env.KMS_KEY_REGION = 'eu-west-1'
      mockCommand.flags = {}

      const mockWallet = { publicKey: new PublicKey(new Uint8Array(32).fill(1)) }
      mockCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as any, mockNext)

      expect(mockCreate).toHaveBeenCalledWith('key-123', 'eu-west-1')
      expect(mockCommand.wallet).toBe(mockWallet)
      expect(mockNext).toHaveBeenCalled()
    })

    it('throws when KMS_KEY_ID is missing', async () => {
      delete process.env.KMS_KEY_ID
      process.env.KMS_KEY_REGION = 'us-east-1'
      mockCommand.flags = { withKms: true }

      await expect(withWallet(mockCommand as any, mockNext)).rejects.toThrow('Missing KMS_KEY_ID environment variable')
      expect(mockCreate).not.toHaveBeenCalled()
    })

    it('throws when KMS_KEY_REGION is missing', async () => {
      process.env.KMS_KEY_ID = 'key-123'
      delete process.env.KMS_KEY_REGION
      mockCommand.flags = { withKms: true }

      await expect(withWallet(mockCommand as any, mockNext)).rejects.toThrow(
        'Missing KMS_KEY_REGION environment variable',
      )
      expect(mockCreate).not.toHaveBeenCalled()
    })
  })
})
