import { PublicKey, Keypair } from '@solana/web3.js'

// Helper to create a mock program with chainable methods builder
const createMockProgram = (instructionResult = {}) => {
  const mockInstruction = jest.fn().mockResolvedValue(instructionResult)
  const mockAccounts = jest.fn().mockReturnValue({ instruction: mockInstruction })
  const createMethodChain = () => ({
    accounts: mockAccounts,
  })
  return {
    methods: {
      addAccess: jest.fn().mockReturnValue(createMethodChain()),
      initialize: jest.fn().mockReturnValue(createMethodChain()),
      setBillingAccessController: jest.fn().mockReturnValue(createMethodChain()),
      setRequesterAccessController: jest.fn().mockReturnValue(createMethodChain()),
      setLoweringAccessController: jest.fn().mockReturnValue(createMethodChain()),
    },
    account: {
      accessController: {
        createInstruction: jest.fn().mockResolvedValue(instructionResult),
      },
      state: {
        fetch: jest.fn().mockResolvedValue({
          config: {
            billingAccessController: new PublicKey(new Uint8Array(32).fill(2)),
            requesterAccessController: new PublicKey(new Uint8Array(32).fill(3)),
          },
        }),
        size: 1000,
      },
      store: {
        fetch: jest.fn().mockResolvedValue({
          loweringAccessController: new PublicKey(new Uint8Array(32).fill(4)),
        }),
      },
    },
    programId: new PublicKey(new Uint8Array(32).fill(5)),
    idl: { errors: [] },
    _mockInstruction: mockInstruction,
    _mockAccounts: mockAccounts,
  }
}

// Mock gauntlet-core
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
    expect: jest.fn(),
  },
  prompt: jest.fn().mockResolvedValue(undefined),
  BN: jest.requireActual('bn.js'),
}))

jest.mock('@chainlink/gauntlet-core/dist/lib/args', () => ({
  boolean: jest.fn((val: string | undefined) => val === 'true'),
}))

// Mock contracts
const mockProgramId = new PublicKey(new Uint8Array(32).fill(10))
jest.mock('../../../src/lib/contracts', () => ({
  CONTRACT_LIST: {
    ACCESS_CONTROLLER: 'access_controller',
    OCR_2: 'ocr_2',
    STORE: 'store',
    TOKEN: 'token',
  },
  getContract: jest.fn().mockReturnValue({
    programId: new PublicKey(new Uint8Array(32).fill(10)),
    idl: { errors: [] },
  }),
}))

// Mock gauntlet-solana SolanaCommand
const mockSignAndSendRawTx = jest.fn().mockResolvedValue('mock-tx-hash')
const mockSendTxWithIDL = jest.fn().mockReturnValue(jest.fn().mockResolvedValue('mock-tx-hash'))
const mockWrapResponse = jest.fn().mockImplementation((hash, address, states) => ({
  hash,
  address,
  states,
  wait: jest.fn().mockResolvedValue({ success: true }),
}))
const mockLoadProgram = jest.fn()

jest.mock('@chainlink/gauntlet-solana', () => {
  class MockSolanaCommand {
    flags: any
    args: any
    wallet: any
    provider: any
    middlewares: any[]
    signAndSendRawTx: any
    sendTxWithIDL: any
    wrapResponse: any
    loadProgram: any

    constructor(flags: any, args: any) {
      this.flags = flags || {}
      this.args = args || []
      this.middlewares = []
      this.wallet = {
        publicKey: new PublicKey(new Uint8Array(32).fill(1)),
      }
      this.provider = {
        connection: {
          getMinimumBalanceForRentExemption: jest.fn().mockResolvedValue(1000000),
        },
      }
      this.signAndSendRawTx = mockSignAndSendRawTx
      this.sendTxWithIDL = mockSendTxWithIDL
      this.wrapResponse = mockWrapResponse
      this.loadProgram = mockLoadProgram
    }

    use() {}

    require(condition: boolean, message: string) {
      if (!condition) throw new Error(message)
    }

    requireFlag(flag: string, message: string) {
      if (!this.flags.help && !this.flags[flag]) throw new Error(message)
    }
  }

  return {
    SolanaCommand: MockSolanaCommand,
    TransactionResponse: {},
  }
})

describe('AddAccess command', () => {
  let AddAccess: any
  let mockProgram: ReturnType<typeof createMockProgram>

  beforeEach(() => {
    jest.clearAllMocks()
    mockProgram = createMockProgram({ programId: mockProgramId, keys: [], data: Buffer.from([]) })
    mockLoadProgram.mockReturnValue(mockProgram)
    jest.isolateModules(() => {
      AddAccess = require('../../../src/commands/contracts/accessController/addAccess').default
    })
  })

  it('has correct static properties', () => {
    expect(AddAccess.id).toBe('access_controller:add_access')
    expect(AddAccess.category).toBe('access_controller')
  })

  it('constructor requires state and address flags', () => {
    expect(() => new AddAccess({}, [])).toThrow()
    expect(() => new AddAccess({ state: 'abc' }, [])).toThrow()
  })

  it('execute calls program.methods.addAccess with correct accounts', async () => {
    const stateKey = 'EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC'
    const accessAddress = '9ohrpVDVNKKW1LipksFrmq6wa1oLLYL9QSoYUn4pAQ2v'
    const cmd = new AddAccess({ state: stateKey, address: accessAddress }, [])
    const result = await cmd.execute()

    expect(mockLoadProgram).toHaveBeenCalled()
    expect(mockProgram.methods.addAccess).toHaveBeenCalled()
    expect(mockProgram._mockAccounts).toHaveBeenCalledWith(
      expect.objectContaining({
        state: new PublicKey(stateKey),
        owner: cmd.wallet.publicKey,
        address: new PublicKey(accessAddress),
      }),
    )
    expect(mockSendTxWithIDL).toHaveBeenCalled()
    expect(result.responses).toHaveLength(1)
  })
})

describe('SetBillingAccessController command', () => {
  let SetBillingAC: any
  let mockProgram: ReturnType<typeof createMockProgram>

  beforeEach(() => {
    jest.clearAllMocks()
    mockProgram = createMockProgram()
    mockLoadProgram.mockReturnValue(mockProgram)
    jest.isolateModules(() => {
      SetBillingAC = require('../../../src/commands/contracts/ocr2/setBillingAccessController').default
    })
  })

  it('has correct static properties', () => {
    expect(SetBillingAC.id).toBe('ocr2:set_billing_access_controller')
    expect(SetBillingAC.category).toBe('ocr_2')
  })

  it('execute calls program.methods.setBillingAccessController', async () => {
    const stateAddr = Keypair.generate().publicKey.toString()
    const acAddr = Keypair.generate().publicKey.toString()
    const cmd = new SetBillingAC({ accessController: acAddr }, [stateAddr])
    const result = await cmd.execute()

    expect(mockProgram.methods.setBillingAccessController).toHaveBeenCalled()
    expect(mockProgram._mockAccounts).toHaveBeenCalledWith(
      expect.objectContaining({
        state: new PublicKey(stateAddr),
        authority: cmd.wallet.publicKey,
        accessController: new PublicKey(acAddr),
      }),
    )
    expect(mockSendTxWithIDL).toHaveBeenCalled()
    expect(result.responses).toHaveLength(1)
  })

  it('throws when new AC equals old AC', async () => {
    const stateAddr = Keypair.generate().publicKey.toString()
    const oldAC = new PublicKey(new Uint8Array(32).fill(2))
    const cmd = new SetBillingAC({ accessController: oldAC.toString() }, [stateAddr])
    await expect(cmd.execute()).rejects.toThrow('same as existing')
  })
})

describe('SetRequesterAccessController command', () => {
  let SetRequesterAC: any
  let mockProgram: ReturnType<typeof createMockProgram>

  beforeEach(() => {
    jest.clearAllMocks()
    mockProgram = createMockProgram()
    mockLoadProgram.mockReturnValue(mockProgram)
    jest.isolateModules(() => {
      SetRequesterAC = require('../../../src/commands/contracts/ocr2/setRequesterAccessController').default
    })
  })

  it('execute calls program.methods.setRequesterAccessController', async () => {
    const stateAddr = Keypair.generate().publicKey.toString()
    const acAddr = Keypair.generate().publicKey.toString()
    const cmd = new SetRequesterAC({ accessController: acAddr }, [stateAddr])
    const result = await cmd.execute()

    expect(mockProgram.methods.setRequesterAccessController).toHaveBeenCalled()
    expect(mockProgram._mockAccounts).toHaveBeenCalledWith(
      expect.objectContaining({
        state: new PublicKey(stateAddr),
        authority: cmd.wallet.publicKey,
        accessController: new PublicKey(acAddr),
      }),
    )
    expect(result.responses).toHaveLength(1)
  })

  it('throws when new AC equals old AC', async () => {
    const stateAddr = Keypair.generate().publicKey.toString()
    const oldAC = new PublicKey(new Uint8Array(32).fill(3))
    const cmd = new SetRequesterAC({ accessController: oldAC.toString() }, [stateAddr])
    await expect(cmd.execute()).rejects.toThrow('same as existing')
  })
})

describe('SetLoweringAccessController command', () => {
  let SetLoweringAC: any
  let mockProgram: ReturnType<typeof createMockProgram>

  beforeEach(() => {
    jest.clearAllMocks()
    mockProgram = createMockProgram()
    mockLoadProgram.mockReturnValue(mockProgram)
    jest.isolateModules(() => {
      SetLoweringAC = require('../../../src/commands/contracts/store/setLoweringAccessController').default
    })
  })

  it('execute calls program.methods.setLoweringAccessController', async () => {
    const stateAddr = Keypair.generate().publicKey.toString()
    const acAddr = Keypair.generate().publicKey.toString()
    const cmd = new SetLoweringAC({ accessController: acAddr }, [stateAddr])
    const result = await cmd.execute()

    expect(mockProgram.methods.setLoweringAccessController).toHaveBeenCalled()
    expect(mockProgram._mockAccounts).toHaveBeenCalledWith(
      expect.objectContaining({
        store: new PublicKey(stateAddr),
        authority: cmd.wallet.publicKey,
        accessController: new PublicKey(acAddr),
      }),
    )
    expect(result.responses).toHaveLength(1)
  })

  it('throws when new AC equals old AC', async () => {
    const stateAddr = Keypair.generate().publicKey.toString()
    const oldAC = new PublicKey(new Uint8Array(32).fill(4))
    const cmd = new SetLoweringAC({ accessController: oldAC.toString() }, [stateAddr])
    await expect(cmd.execute()).rejects.toThrow('same as existing')
  })
})
