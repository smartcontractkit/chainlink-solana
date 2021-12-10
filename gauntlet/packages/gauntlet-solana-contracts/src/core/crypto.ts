import { keccak256 as eth_keccak256 } from '@ethersproject/keccak256'
import { createCipheriv } from 'crypto'
import { scalarMult, scalarMultBase } from './scalarMult'

export function compute_x_oracles(x_signers: Buffer, x_proposer: Buffer): Buffer {
  const len = x_signers.length + x_proposer.length + 1
  const buffer = Buffer.concat([x_signers, x_proposer, Buffer.from([0])], len)
  return keccak256(buffer).slice(0, 16)
}

export function x25519_keypair(x_signers: Buffer, x_proposer: Buffer): { publicKey: Buffer; secretKey: Buffer } {
  const len = x_signers.length + x_proposer.length + 1
  const buffer = Buffer.concat([x_signers, x_proposer, Buffer.from([1])], len)
  const sk = keccak256(buffer)
  return { publicKey: Buffer.from(scalarMultBase(sk)), secretKey: sk }
}

export function compute_key_i(pk_i: Buffer, sk: Buffer): Buffer {
  const sclMult = Buffer.from(scalarMult(sk, pk_i))
  return keccak256(sclMult).slice(0, 16)
}

export function keccak256(buffer: string | Buffer): Buffer {
  const hexStr = eth_keccak256(buffer)
  const trimmed = hexStr.substr(2)
  return Buffer.from(trimmed, 'hex')
}

export function compute_enc_i(key_i: Buffer, x_oracles: Buffer): Buffer {
  const cipher = createCipheriv('AES-128-ECB', key_i, null)
  const encrypted = cipher.update(x_oracles)
  return Buffer.concat([encrypted, cipher.final()], 16)
}
