import { Keypair, PublicKey } from '@solana/web3.js'
import { WalletTypes } from './wallet'

// Mock KMS wallet
const mockKmsPublicKey = new PublicKey(new Uint8Array(32).fill(3))
jest.mock('./kmsWallet', () => ({
  KMSWallet: {
    create: jest.fn().mockResolvedValue({
      publicKey: new PublicKey(new Uint8Array(32).fill(3)),
      type: () => 'kms',
      signTransaction: jest.fn(),
    }),
  },
}))

// Mock Ledger dependencies
jest.mock('@ledgerhq/hw-transport-node-hid', () => ({
  default: { create: jest.fn() },
}))
jest.mock('@ledgerhq/hw-app-solana', () => {
  return jest.fn()
})

// Mock wallet module — use inline values to avoid hoisting issues
jest.mock('./wallet', () => ({
  ...jest.requireActual('./wallet'),
  LocalWallet: {
    create: jest.fn().mockResolvedValue({
      publicKey: new PublicKey(new Uint8Array(32).fill(1)),
      type: () => 'local',
      signTransaction: jest.fn(),
    }),
  },
  LedgerWallet: {
    create: jest.fn().mockResolvedValue({
      publicKey: new PublicKey(new Uint8Array(32).fill(2)),
      type: () => 'ledger',
      signTransaction: jest.fn(),
    }),
  },
}))

// Mock logger/assertions
jest.mock('@chainlink/gauntlet-core/dist/utils', () => ({
  logger: { info: jest.fn(), warn: jest.fn(), error: jest.fn() },
  assertions: {
    assert: jest.fn((condition: boolean, message: string) => {
      if (!condition) throw new Error(message)
    }),
  },
}))

jest.mock('@chainlink/gauntlet-core/dist/lib/args', () => ({
  boolean: jest.fn((val: string | undefined) => val === 'true'),
}))

// Mock AnchorProvider
jest.mock('@coral-xyz/anchor', () => ({
  AnchorProvider: jest.fn().mockImplementation(() => ({})),
}))

import { withWallet } from './middlewares'
import { KMSWallet } from './kmsWallet'
import { LocalWallet, LedgerWallet } from './wallet'

describe('withWallet middleware', () => {
  const originalEnv = process.env
  let mockCommand: any
  let mockNext: jest.Mock

  beforeEach(() => {
    jest.clearAllMocks()
    process.env = { ...originalEnv }
    mockNext = jest.fn()
    mockCommand = {
      flags: {},
      wallet: null,
    }
  })

  afterAll(() => {
    process.env = originalEnv
  })

  it('loads KMS wallet when --withKms flag is set', async () => {
    mockCommand.flags = { withKms: true }
    process.env.KMS_KEY_ID = 'test-key-id'
    process.env.KMS_KEY_REGION = 'us-east-1'

    await withWallet(mockCommand, mockNext)

    expect(KMSWallet.create).toHaveBeenCalledWith('test-key-id', 'us-east-1')
    expect(mockCommand.wallet).toBeDefined()
    expect(mockNext).toHaveBeenCalled()
  })

  it('loads KMS wallet when WITH_KMS env var is set', async () => {
    process.env.WITH_KMS = 'true'
    process.env.KMS_KEY_ID = 'test-key-id'
    process.env.KMS_KEY_REGION = 'us-east-1'

    await withWallet(mockCommand, mockNext)

    expect(KMSWallet.create).toHaveBeenCalledWith('test-key-id', 'us-east-1')
    expect(mockNext).toHaveBeenCalled()
  })

  it('throws when KMS_KEY_ID is missing', async () => {
    mockCommand.flags = { withKms: true }
    process.env.KMS_KEY_REGION = 'us-east-1'
    delete process.env.KMS_KEY_ID

    await expect(withWallet(mockCommand, mockNext)).rejects.toThrow('Missing KMS_KEY_ID')
  })

  it('throws when KMS_KEY_REGION is missing', async () => {
    mockCommand.flags = { withKms: true }
    process.env.KMS_KEY_ID = 'test-key-id'
    delete process.env.KMS_KEY_REGION

    await expect(withWallet(mockCommand, mockNext)).rejects.toThrow('Missing KMS_KEY_REGION')
  })

  it('loads Local wallet when no flags are set', async () => {
    const keypair = Keypair.generate()
    process.env.PRIVATE_KEY = JSON.stringify(Array.from(keypair.secretKey))

    await withWallet(mockCommand, mockNext)

    expect(LocalWallet.create).toHaveBeenCalled()
    expect(mockNext).toHaveBeenCalled()
  })

  it('prioritizes Ledger over KMS', async () => {
    mockCommand.flags = { withLedger: true, withKms: true }
    process.env.KMS_KEY_ID = 'test-key-id'
    process.env.KMS_KEY_REGION = 'us-east-1'

    await withWallet(mockCommand, mockNext)

    expect(LedgerWallet.create).toHaveBeenCalled()
    expect(KMSWallet.create).not.toHaveBeenCalled()
  })
})
