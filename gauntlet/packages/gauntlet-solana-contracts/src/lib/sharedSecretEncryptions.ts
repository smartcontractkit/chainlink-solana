import * as crypto from './crypto'

export type SharedSecretEncryptions = {
  diffieHellmanPoint: Buffer
  sharedSecretHash: Buffer
  encryptions: Buffer[]
}

export const makeSharedSecretEncryptions = (
  x_signersWords: string,
  x_proposerWords: string,
  nodes: string[],
): SharedSecretEncryptions => {
  const x_signers = crypto.keccak256(Buffer.from(x_signersWords))
  const x_proposer = crypto.keccak256(Buffer.from(x_proposerWords))
  const x_oracles = crypto.compute_x_oracles(x_signers, x_proposer)
  const { publicKey, secretKey } = crypto.x25519_keypair(x_signers, x_proposer)
  const encryptions: Buffer[] = nodes.map((node) => {
    const pk_i = Buffer.from(node, 'hex')
    const key_i = crypto.compute_key_i(pk_i, secretKey)
    return crypto.compute_enc_i(key_i, x_oracles)
  })

  return {
    diffieHellmanPoint: publicKey,
    sharedSecretHash: crypto.keccak256(x_oracles),
    encryptions,
  }
}
