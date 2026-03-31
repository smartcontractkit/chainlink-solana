import { TransactionInstruction, PublicKey, ComputeBudgetProgram } from '@solana/web3.js'
import { makeTx, percentile, validateHistoricalPriorityFeeInput, validateRetryPriorityInput } from './utils'

describe('makeTx', () => {
  const dummyInstruction: TransactionInstruction = {
    keys: [],
    programId: new PublicKey(new Uint8Array(32).fill(1)),
    data: Buffer.from([]),
  }

  it('creates a transaction with provided instructions', () => {
    const tx = makeTx([dummyInstruction])
    // Transaction should contain the instruction
    expect(tx.instructions).toHaveLength(1)
    expect(tx.instructions[0].programId).toEqual(dummyInstruction.programId)
  })

  it('creates a transaction with multiple instructions', () => {
    const tx = makeTx([dummyInstruction, dummyInstruction])
    expect(tx.instructions).toHaveLength(2)
  })

  it('creates empty transaction when no instructions', () => {
    const tx = makeTx([])
    expect(tx.instructions).toHaveLength(0)
  })

  it('adds compute unit price when overrides.price is set', () => {
    const tx = makeTx([dummyInstruction], undefined, { price: 1000 })
    // Should have 2 instructions: compute budget + the instruction
    expect(tx.instructions).toHaveLength(2)
  })

  it('adds compute unit limit when overrides.units is set', () => {
    const tx = makeTx([dummyInstruction], undefined, { units: 200000 })
    expect(tx.instructions).toHaveLength(2)
  })

  it('adds both compute budget instructions when both overrides are set', () => {
    const tx = makeTx([dummyInstruction], undefined, { price: 1000, units: 200000 })
    expect(tx.instructions).toHaveLength(3)
  })

  it('does not add compute budget when overrides are empty', () => {
    const tx = makeTx([dummyInstruction], undefined, {})
    expect(tx.instructions).toHaveLength(1)
  })
})

describe('percentile', () => {
  it('returns null for empty array', () => {
    expect(percentile([], 0.5)).toBeNull()
  })

  it('returns the single value for single-element array', () => {
    expect(percentile([42], 0.5)).toBe(42)
  })

  it('returns min value for percentile 0', () => {
    expect(percentile([10, 20, 30], 0)).toBe(10)
  })

  it('returns max value for percentile 1', () => {
    expect(percentile([10, 20, 30], 1)).toBe(30)
  })

  it('returns median for percentile 0.5', () => {
    expect(percentile([10, 20, 30], 0.5)).toBe(20)
  })

  it('interpolates between values', () => {
    const result = percentile([10, 20], 0.5)
    expect(result).toBe(15)
  })

  it('handles unsorted input', () => {
    expect(percentile([30, 10, 20], 0.5)).toBe(20)
  })

  it('throws for percentile below 0', () => {
    expect(() => percentile([1, 2, 3], -0.1)).toThrow('Percentile must be between 0 and 1')
  })

  it('throws for percentile above 1', () => {
    expect(() => percentile([1, 2, 3], 1.1)).toThrow('Percentile must be between 0 and 1')
  })
})

describe('validateHistoricalPriorityFeeInput', () => {
  it('accepts valid input', () => {
    expect(validateHistoricalPriorityFeeInput('0.5,10')).toBeUndefined()
  })

  it('accepts edge case values', () => {
    expect(validateHistoricalPriorityFeeInput('0.1,1')).toBeUndefined()
  })

  it('throws for empty input', () => {
    expect(() => validateHistoricalPriorityFeeInput('')).toThrow('No Input')
  })

  it('throws when percentile is >= 1', () => {
    expect(() => validateHistoricalPriorityFeeInput('1,10')).toThrow('Invalid input')
  })

  it('throws when nBlocks is 0', () => {
    expect(() => validateHistoricalPriorityFeeInput('0.5,0')).toThrow('Invalid input')
  })

  it('throws for non-numeric input', () => {
    expect(() => validateHistoricalPriorityFeeInput('abc,def')).toThrow('Invalid input')
  })
})

describe('validateRetryPriorityInput', () => {
  it('accepts valid input', () => {
    expect(validateRetryPriorityInput('0.1,3')).toBeUndefined()
  })

  it('throws for empty input', () => {
    expect(() => validateRetryPriorityInput('')).toThrow('No Input')
  })

  it('throws when bump percentage is >= 1', () => {
    expect(() => validateRetryPriorityInput('1,3')).toThrow('Invalid input')
  })

  it('throws when number of retries is 0', () => {
    expect(() => validateRetryPriorityInput('0.1,0')).toThrow('Invalid input')
  })

  it('throws for non-numeric input', () => {
    expect(() => validateRetryPriorityInput('abc,def')).toThrow('Invalid input')
  })
})
