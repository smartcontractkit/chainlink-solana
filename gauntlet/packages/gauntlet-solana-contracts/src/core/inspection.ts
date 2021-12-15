import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey } from '@solana/web3.js'
import BN from 'bn.js'
import { assert } from './assert'
import { Protobuf } from './proto'

export type Inspection = {
  value: any
  expected: any
  name: string
}

export const makeInspection = (value, expected, name) => ({ value, expected, name })

export const inspect = (inspections: Inspection[]): boolean => {
  let success = true
  inspections.forEach(({ value, expected, name }) => {
    if (!assert(value, expected)) {
      success = false
      logger.warn(`${name} invalid: expected ${expected} but actually ${value}`)
    } else {
      logger.success(`${name} matches: ${expected}`)
    }
  })
  return success
}

export const toComparableNumber = (v: string | number) => new BN(v).toString()
export const toComparableLongNumber = (v: Long) => new BN(Protobuf.longToString(v)).toString()

export const toComparablePubKey = (v: string) => {
  try {
    return new PublicKey(v).toString()
  } catch (e) {
    logger.error(`Error generating public key from ${v}`)
    return ''
  }
}
