interface Oracle {
  signer: Buffer
  transmitter: Buffer
}

export interface OCR2Config {
  oracles: Oracle[]
  threshold: number
  onchainConfig: Buffer
  offchainConfig: Buffer
  offchainConfigVersion: number
}
