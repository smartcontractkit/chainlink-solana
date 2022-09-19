import { getAccount } from '@solana/spl-token'
import { Connection, PublicKey } from '@solana/web3.js'

export const isValidTokenAccount = async (connection: Connection, token: PublicKey, address: PublicKey) => {
  try {
    const info = await getAccount(connection, address)
    return info.mint == token && !!info.address
  } catch (e) {
    return false
  }
}
