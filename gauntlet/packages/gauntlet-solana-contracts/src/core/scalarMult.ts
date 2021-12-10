// This file contains vendoring of tweetnacl-js' scalarMult
// https://github.com/dchest/tweetnacl-js/blob/f1ec050ceae0861f34280e62498b1d3ed9c350c6/nacl.js

export const crypto_scalarmult_SCALARBYTES = 32
export const crypto_scalarmult_BYTES = 32

export const scalarMult = (n: Uint8Array, p: Uint8Array): Uint8Array => {
  if (n.length !== crypto_scalarmult_SCALARBYTES) throw new Error('bad n size')
  if (p.length !== crypto_scalarmult_BYTES) throw new Error('bad p size')
  let q = new Uint8Array(crypto_scalarmult_BYTES)
  crypto_scalarmult(q, n, p)
  return q
}

export const scalarMultBase = (n: Uint8Array): Uint8Array => {
  if (n.length !== crypto_scalarmult_SCALARBYTES) throw new Error('bad n size')
  const q = new Uint8Array(crypto_scalarmult_BYTES)
  crypto_scalarmult_base(q, n)
  return q
}

const gf = (init?: number[]): Float64Array => {
  const r = new Float64Array(16)
  if (init) for (let i = 0; i < init.length; i++) r[i] = init[i]
  return r
}

const unpack25519 = (o: Float64Array, n: Uint8Array): void => {
  for (let i = 0; i < 16; i++) o[i] = n[2 * i] + (n[2 * i + 1] << 8)
  o[15] &= 0x7fff
}

const car25519 = (o: Float64Array): void => {
  for (let i = 0; i < 16; i++) {
    o[i] += 65536
    const c = Math.floor(o[i] / 65536)
    o[(i + 1) * (i < 15 ? 1 : 0)] += c - 1 + 37 * (c - 1) * (i === 15 ? 1 : 0)
    o[i] -= c * 65536
  }
}

const sel25519 = (p: Float64Array, q: Float64Array, b: number): void => {
  const c = ~(b - 1)
  for (let i = 0; i < 16; i++) {
    const t = c & (p[i] ^ q[i])
    p[i] ^= t
    q[i] ^= t
  }
}

const A = (o: Float64Array, a: Float64Array, b: Float64Array): void => {
  for (let i = 0; i < 16; i++) o[i] = (a[i] + b[i]) | 0
}

const Z = (o: Float64Array, a: Float64Array, b: Float64Array): void => {
  for (let i = 0; i < 16; i++) o[i] = (a[i] - b[i]) | 0
}

const M = (o: Float64Array, a: Float64Array, b: Float64Array): void => {
  const t = new Float64Array(31)
  for (let i = 0; i < 31; i++) t[i] = 0
  for (let i = 0; i < 16; i++) {
    for (let j = 0; j < 16; j++) {
      t[i + j] += a[i] * b[j]
    }
  }
  for (let i = 0; i < 15; i++) {
    t[i] += 38 * t[i + 16]
  }
  for (let i = 0; i < 16; i++) o[i] = t[i]
  car25519(o)
  car25519(o)
}

const S = (o: Float64Array, a: Float64Array) => M(o, a, a)

const inv25519 = (o: Float64Array, i: Float64Array): void => {
  const c = gf()
  for (let a = 0; a < 16; a++) c[a] = i[a]
  for (let a = 253; a >= 0; a--) {
    S(c, c)
    if (a !== 2 && a !== 4) M(c, c, i)
  }
  for (let a = 0; a < 16; a++) o[a] = c[a]
}

const pack25519 = (o: Uint8Array, n: Float64Array): void => {
  const m = gf(),
    t = gf()
  for (let i = 0; i < 16; i++) t[i] = n[i]
  car25519(t)
  car25519(t)
  car25519(t)
  for (let j = 0; j < 2; j++) {
    m[0] = t[0] - 0xffed
    for (let i = 1; i < 15; i++) {
      m[i] = t[i] - 0xffff - ((m[i - 1] >> 16) & 1)
      m[i - 1] &= 0xffff
    }
    m[15] = t[15] - 0x7fff - ((m[14] >> 16) & 1)
    const b = (m[15] >> 16) & 1
    m[14] &= 0xffff
    sel25519(t, m, 1 - b)
  }
  for (let i = 0; i < 16; i++) {
    o[2 * i] = t[i] & 0xff
    o[2 * i + 1] = t[i] >> 8
  }
}

const _121665 = gf([0xdb41, 1])
const _9 = new Uint8Array(32)
_9[0] = 9

const crypto_scalarmult = (q: Uint8Array, n: Uint8Array, p: Uint8Array): number => {
  const z = new Uint8Array(32)
  const x = new Float64Array(80)
  const a = gf(),
    b = gf(),
    c = gf(),
    d = gf(),
    e = gf(),
    f = gf()
  for (let i = 0; i < 31; i++) z[i] = n[i]
  z[31] = (n[31] & 127) | 64
  z[0] &= 248
  unpack25519(x, p)
  for (let i = 0; i < 16; i++) {
    b[i] = x[i]
    d[i] = a[i] = c[i] = 0
  }
  a[0] = d[0] = 1
  for (let i = 254; i >= 0; --i) {
    const r = (z[i >>> 3] >>> (i & 7)) & 1
    sel25519(a, b, r)
    sel25519(c, d, r)
    A(e, a, c)
    Z(a, a, c)
    A(c, b, d)
    Z(b, b, d)
    S(d, e)
    S(f, a)
    M(a, c, a)
    M(c, b, e)
    A(e, a, c)
    Z(a, a, c)
    S(b, a)
    Z(c, d, f)
    M(a, c, _121665)
    A(a, a, d)
    M(c, c, a)
    M(a, d, f)
    M(d, b, x)
    S(b, e)
    sel25519(a, b, r)
    sel25519(c, d, r)
  }
  for (let i = 0; i < 16; i++) {
    x[i + 16] = a[i]
    x[i + 32] = c[i]
    x[i + 48] = b[i]
    x[i + 64] = d[i]
  }
  const x32 = x.subarray(32)
  const x16 = x.subarray(16)
  inv25519(x32, x32)
  M(x16, x16, x32)
  pack25519(q, x16)
  return 0
}

const crypto_scalarmult_base = (q: Uint8Array, n: Uint8Array): number => crypto_scalarmult(q, n, _9)
