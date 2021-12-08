import Solana from '../../src/commands'

describe('Command', () => {
  it('Load Solana commands', () => {
    expect(Solana.length).toBeGreaterThan(0)
  })
})
