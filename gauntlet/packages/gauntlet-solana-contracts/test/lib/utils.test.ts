import { encodeInstruction, divideIntoChunks } from '../../src/lib/utils'

describe('encodeInstruction', () => {
  it('encodes Upgrade instruction', () => {
    const result = encodeInstruction({ Upgrade: {} })
    expect(result).toBeInstanceOf(Buffer)
    expect(result.length).toBeGreaterThan(0)
    expect(result.length).toBeLessThanOrEqual(4 + 4 + 8 + 900)
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
})
