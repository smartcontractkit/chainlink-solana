import { Token } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'

export const isValidTokenAccount = async (token: Token, address: PublicKey) => {
  try {
    const info = await token.getAccountInfo(address)
    return !!info.address
  } catch (e) {
    return false
  }
}
