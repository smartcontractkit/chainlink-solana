import { encodeInstruction, divideIntoChunks } from '../../src/lib/utils'

describe('encodeInstruction', () => {
  it('encodes Upgrade instruction', () => {
    const result = encodeInstruction({ Upgrade: {} })
    expect(result).toBeInstanceOf(Buffer)
    expect(result.length).toBeGreaterThan(0)
    expect(result.length).toBeLessThanOrEqual(4 + 4 + 8 + 900)
  })

  it('encodes InitializeBuffer instruction', () => {
    const result = encodeInstruction({ InitializeBuffer: {} })
    expect(result).toBeInstanceOf(Buffer)
    expect(result.length).toBeGreaterThan(0)
  })

  it('encodes SetAuthority instruction', () => {
    const result = encodeInstruction({ SetAuthority: {} })
    expect(result).toBeInstanceOf(Buffer)
    expect(result.length).toBeGreaterThan(0)
  })

  it('encodes Close instruction', () => {
    const result = encodeInstruction({ Close: {} })
    expect(result).toBeInstanceOf(Buffer)
    expect(result.length).toBeGreaterThan(0)
  })
})

describe('divideIntoChunks', () => {
  it('splits array into chunks of specified size', () => {
    const arr = [1, 2, 3, 4, 5, 6]
    const chunks = divideIntoChunks(arr, 2)
    expect(chunks).toEqual([
      [1, 2],
      [3, 4],
      [5, 6],
    ])
  })

  it('handles array not evenly divisible by chunk size', () => {
    const arr = [1, 2, 3, 4, 5]
    const chunks = divideIntoChunks(arr, 2)
    expect(chunks).toEqual([[1, 2], [3, 4], [5]])
  })

  it('handles empty array', () => {
    const chunks = divideIntoChunks([], 3)
    expect(chunks).toEqual([])
  })

  it('handles chunk size larger than array', () => {
    const arr = [1, 2]
    const chunks = divideIntoChunks(arr, 10)
    expect(chunks).toEqual([[1, 2]])
  })

  it('handles Buffer input', () => {
    const buf = Buffer.from([1, 2, 3, 4])
    const chunks = divideIntoChunks(buf, 2)
    expect(chunks).toHaveLength(2)
  })

  it('handles chunk size of 1', () => {
    const arr = [1, 2, 3]
    const chunks = divideIntoChunks(arr, 1)
    expect(chunks).toEqual([[1], [2], [3]])
  })
})
