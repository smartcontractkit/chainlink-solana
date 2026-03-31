import { withWallet, withProvider, withNetwork } from './middlewares'
import { KMSWallet } from './kmsWallet'
import { LedgerWallet, LocalWallet } from './wallet'
import { PublicKey, Keypair } from '@solana/web3.js'

jest.mock('./kmsWallet', () => ({
  KMSWallet: {
    create: jest.fn(),
  },
}))

jest.mock('./wallet', () => ({
  LedgerWallet: {
    create: jest.fn(),
  },
  LocalWallet: {
    create: jest.fn(),
  },
}))

jest.mock('@coral-xyz/anchor', () => ({
  AnchorProvider: jest.fn().mockImplementation(() => ({ connection: {} })),
}))

jest.mock('@solana/web3.js', () => {
  const actual = jest.requireActual('@solana/web3.js')
  return {
    ...actual,
    Connection: jest.fn().mockImplementation(() => ({})),
  }
})

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
      if (!condition) {
        throw new Error(message)
      }
    }),
  },
}))

jest.mock('@chainlink/gauntlet-core/dist/lib/args', () => ({
  boolean: jest.fn((val: string | undefined) => val === 'true'),
}))

const mockKmsCreate = KMSWallet.create as jest.Mock
const mockLedgerCreate = LedgerWallet.create as unknown as jest.Mock
const mockLocalCreate = LocalWallet.create as unknown as jest.Mock

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
      mockKmsCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)

      expect(mockKmsCreate).toHaveBeenCalledWith('arn:aws:kms:us-east-1:123:key/abc', 'us-east-1')
      expect(mockCommand.wallet).toBe(mockWallet)
      expect(mockNext).toHaveBeenCalled()
    })

    it('creates KMS wallet when WITH_KMS env is set', async () => {
      process.env.WITH_KMS = 'true'
      process.env.KMS_KEY_ID = 'key-123'
      process.env.KMS_KEY_REGION = 'eu-west-1'
      mockCommand.flags = {}

      const mockWallet = { publicKey: new PublicKey(new Uint8Array(32).fill(1)) }
      mockKmsCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)

      expect(mockKmsCreate).toHaveBeenCalledWith('key-123', 'eu-west-1')
      expect(mockCommand.wallet).toBe(mockWallet)
      expect(mockNext).toHaveBeenCalled()
    })

    it('throws when KMS_KEY_ID is missing', async () => {
      delete process.env.KMS_KEY_ID
      process.env.KMS_KEY_REGION = 'us-east-1'
      mockCommand.flags = { withKms: true }

      await expect(withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)).rejects.toThrow(
        'Missing KMS_KEY_ID environment variable',
      )
      expect(mockKmsCreate).not.toHaveBeenCalled()
    })

    it('throws when KMS_KEY_REGION is missing', async () => {
      process.env.KMS_KEY_ID = 'key-123'
      delete process.env.KMS_KEY_REGION
      mockCommand.flags = { withKms: true }

      await expect(withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)).rejects.toThrow(
        'Missing KMS_KEY_REGION environment variable',
      )
      expect(mockKmsCreate).not.toHaveBeenCalled()
    })
  })

  describe('Ledger path', () => {
    it('creates Ledger wallet when --withLedger flag is set', async () => {
      mockCommand.flags = { withLedger: true }

      const mockWallet = { publicKey: new PublicKey(new Uint8Array(32).fill(3)) }
      mockLedgerCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)

      expect(mockLedgerCreate).toHaveBeenCalledWith("44'/501'")
      expect(mockCommand.wallet).toBe(mockWallet)
      expect(mockNext).toHaveBeenCalled()
    })

    it('uses custom ledgerPath when provided', async () => {
      mockCommand.flags = { withLedger: true, ledgerPath: "44'/501'/0'" }

      const mockWallet = { publicKey: new PublicKey(new Uint8Array(32).fill(3)) }
      mockLedgerCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)

      expect(mockLedgerCreate).toHaveBeenCalledWith("44'/501'/0'")
    })

    it('Ledger takes priority over KMS', async () => {
      process.env.KMS_KEY_ID = 'key-123'
      process.env.KMS_KEY_REGION = 'us-east-1'
      mockCommand.flags = { withLedger: true, withKms: true }

      const mockWallet = { publicKey: new PublicKey(new Uint8Array(32).fill(3)) }
      mockLedgerCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)

      expect(mockLedgerCreate).toHaveBeenCalled()
      expect(mockKmsCreate).not.toHaveBeenCalled()
    })
  })

  describe('Local wallet path', () => {
    it('creates Local wallet from PRIVATE_KEY env', async () => {
      const keypair = Keypair.generate()
      process.env.PRIVATE_KEY = JSON.stringify(Array.from(keypair.secretKey))
      mockCommand.flags = {}

      const mockWallet = { publicKey: keypair.publicKey }
      mockLocalCreate.mockResolvedValue(mockWallet)

      await withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)

      expect(mockLocalCreate).toHaveBeenCalled()
      expect(mockCommand.wallet).toBe(mockWallet)
      expect(mockNext).toHaveBeenCalled()
    })

    it('throws when PRIVATE_KEY is missing', async () => {
      delete process.env.PRIVATE_KEY
      mockCommand.flags = {}

      await expect(withWallet(mockCommand as Parameters<typeof withWallet>[0], mockNext)).rejects.toThrow(
        'Missing PRIVATE_KEY',
      )
      expect(mockLocalCreate).not.toHaveBeenCalled()
    })
  })
})

describe('withProvider', () => {
  const originalEnv = process.env
  let mockNext: jest.Mock
  let mockCommand: { flags: Record<string, unknown>; wallet?: unknown; provider?: unknown }

  beforeEach(() => {
    jest.clearAllMocks()
    process.env = { ...originalEnv }
    mockNext = jest.fn().mockResolvedValue(undefined)
    mockCommand = {
      flags: {},
      wallet: { publicKey: new PublicKey(new Uint8Array(32).fill(1)) },
    }
  })

  afterEach(() => {
    process.env = originalEnv
  })

  it('sets provider when NODE_URL is a valid http URL', () => {
    process.env.NODE_URL = 'http://localhost:8899'

    withProvider(mockCommand as Parameters<typeof withProvider>[0], mockNext)

    expect(mockCommand.provider).toBeDefined()
    expect(mockNext).toHaveBeenCalled()
  })

  it('sets provider when NODE_URL is a valid https URL', () => {
    process.env.NODE_URL = 'https://api.mainnet-beta.solana.com'

    withProvider(mockCommand as Parameters<typeof withProvider>[0], mockNext)

    expect(mockCommand.provider).toBeDefined()
    expect(mockNext).toHaveBeenCalled()
  })

  it('throws when NODE_URL is missing', () => {
    delete process.env.NODE_URL

    expect(() => withProvider(mockCommand as Parameters<typeof withProvider>[0], mockNext)).toThrow('Invalid NODE_URL')
  })

  it('throws when NODE_URL has no protocol prefix', () => {
    process.env.NODE_URL = 'localhost:8899'

    expect(() => withProvider(mockCommand as Parameters<typeof withProvider>[0], mockNext)).toThrow('Invalid NODE_URL')
  })

  it('accepts WS_URL with ws:// prefix', () => {
    process.env.NODE_URL = 'http://localhost:8899'
    process.env.WS_URL = 'ws://localhost:8900'

    withProvider(mockCommand as Parameters<typeof withProvider>[0], mockNext)

    expect(mockCommand.provider).toBeDefined()
    expect(mockNext).toHaveBeenCalled()
  })

  it('throws when WS_URL has invalid prefix', () => {
    process.env.NODE_URL = 'http://localhost:8899'
    process.env.WS_URL = 'localhost:8900'

    expect(() => withProvider(mockCommand as Parameters<typeof withProvider>[0], mockNext)).toThrow('Invalid WS_URL')
  })
})

describe('withNetwork', () => {
  let mockNext: jest.Mock
  let mockCommand: { flags: Record<string, unknown> }

  beforeEach(() => {
    jest.clearAllMocks()
    mockNext = jest.fn().mockResolvedValue(undefined)
    mockCommand = {
      flags: {},
    }
  })

  it('passes when --network is set', () => {
    mockCommand.flags = { network: 'devnet' }

    withNetwork(mockCommand as Parameters<typeof withNetwork>[0], mockNext)

    expect(mockNext).toHaveBeenCalled()
  })

  it('throws when --network is missing', () => {
    mockCommand.flags = {}

    expect(() => withNetwork(mockCommand as Parameters<typeof withNetwork>[0], mockNext)).toThrow('Network required')
  })
})
