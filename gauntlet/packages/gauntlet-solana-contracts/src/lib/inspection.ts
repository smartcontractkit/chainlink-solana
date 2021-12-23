import { BN, logger } from '@chainlink/gauntlet-core/dist/utils'
import { Proto } from '@chainlink/gauntlet-core/dist/crypto'
import { PublicKey } from '@solana/web3.js'

export const toComparableNumber = (v: string | number) => new BN(v).toString()
export const toComparableLongNumber = (v: Long) => new BN(Proto.Protobuf.longToString(v)).toString()

export const toComparablePubKey = (v: string) => {
  try {
    return new PublicKey(v).toString()
  } catch (e) {
    logger.error(`Error generating public key from ${v}`)
    return ''
  }
}
