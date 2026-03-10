import { PublicKey } from '@solana/web3.js'
import { makeRawUpgradeTransaction } from '../../../src/commands/abstract/upgrade'
import { CONTRACT_LIST } from '../../../src/lib/contracts'
import { UPGRADEABLE_BPF_LOADER_PROGRAM_ID } from '../../../src/lib/constants'

jest.mock('../../../src/lib/contracts', () => ({
  CONTRACT_LIST: {
    ACCESS_CONTROLLER: 'access_controller',
    OCR_2: 'ocr_2',
    STORE: 'store',
    TOKEN: 'token',
  },
  getContract: jest.fn(),
}))

const { getContract } = jest.requireMock('../../../src/lib/contracts')

describe('makeRawUpgradeTransaction', () => {
  const mockProgramId = new PublicKey('EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC')
  const mockProgramDataKey = new PublicKey('11111111111111111111111111111111')
  const mockSigner = new PublicKey('9ohrpVDVNKKW1LipksFrmq6wa1oLLYL9QSoYUn4pAQ2v')
  const mockBufferAccount = 'EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC'

  beforeEach(() => {
    jest.clearAllMocks()
    ;(getContract as jest.Mock).mockReturnValue({
      id: CONTRACT_LIST.ACCESS_CONTROLLER,
      programId: mockProgramId,
    })
    jest.spyOn(PublicKey, 'findProgramAddress').mockResolvedValue([mockProgramDataKey, 0])
  })

  afterEach(() => {
    jest.restoreAllMocks()
  })

  it('returns upgrade instruction with correct structure', async () => {
    const result = await makeRawUpgradeTransaction(mockSigner, CONTRACT_LIST.ACCESS_CONTROLLER, mockBufferAccount)

    expect(result).toHaveLength(1)
    expect(result[0].programId).toEqual(UPGRADEABLE_BPF_LOADER_PROGRAM_ID)
    expect(result[0].keys).toHaveLength(7)
    expect(result[0].keys.map((k) => k.pubkey.toString())).toContain(mockProgramDataKey.toString())
    expect(result[0].keys.map((k) => k.pubkey.toString())).toContain(mockProgramId.toString())
    expect(result[0].keys.map((k) => k.pubkey.toString())).toContain(mockSigner.toString())
    expect(result[0].data).toBeInstanceOf(Buffer)
    expect(result[0].data.length).toBeGreaterThan(0)
  })

  it('uses signer as both account and signer in keys', async () => {
    const result = await makeRawUpgradeTransaction(mockSigner, CONTRACT_LIST.STORE, mockBufferAccount)

    const signerEntries = result[0].keys.filter((k) => k.pubkey.equals(mockSigner))
    expect(signerEntries).toHaveLength(2)
    const signerKey = result[0].keys.find((k) => k.pubkey.equals(mockSigner) && k.isSigner)
    expect(signerKey?.isSigner).toBe(true)
  })
})
