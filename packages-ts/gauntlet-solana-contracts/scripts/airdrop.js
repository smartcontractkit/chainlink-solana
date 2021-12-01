const { Connection, PublicKey, LAMPORTS_PER_SOL } = require('@solana/web3.js')

const connection = new Connection('https://api.devnet.solana.com', 'confirmed')
const myAddress = new PublicKey('9ohrpVDVNKKW1LipksFrmq6wa1oLLYL9QSoYUn4pAQ2v')

const requestAirdrop = async () => {
  const signature = await connection.requestAirdrop(myAddress, LAMPORTS_PER_SOL)
  await connection.confirmTransaction(signature)
}

const sleep = (ms) => {
  return new Promise((resolve) =>
    setTimeout(function () {
      resolve()
    }, ms),
  )
}

const infiniteRequest = async () => {
  while (true) {
    console.log('Requesting 1 SOL')
    await requestAirdrop()
    await sleep(10 * 1000)
  }
}

;(async () => {
  await infiniteRequest()
})()
