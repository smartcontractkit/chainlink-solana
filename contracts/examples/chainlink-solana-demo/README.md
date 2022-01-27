# chainlink-solana

## Running the example on devnet

Generate a new wallet:

```
solana-keygen new -o id.json
solana-keygen pubkey id.json
```

Airdrop some tokens:

```
solana airdrop 4 <RECIPIENT_ACCOUNT_ADDRESS> --url https://api.devnet.solana.com
```

Build and deploy the contract:

```
anchor build
anchor deploy --provider.cluster devnet
```

Extract the `program_id` and insert it into `client.js`

```
solana-keygen pubkey target/deploy/hello_world-keypair.json
```

Now run the Node.JS client:

```
ANCHOR_PROVIDER_URL=https://api.devnet.solana.com ANCHOR_WALLET=id.json node client.js
Running client...
Fetching transaction logs...
[
  'Program <PROGRAM_ID> invoke [1]',
  'Program log: Instruction: Execute',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g invoke [2]',
  'Program log: Instruction: Query',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g consumed 2916 of 196135 compute units',
  'Program return: DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g 6NQDALZQ7mEAAAAAQH32BQAAAAAAAAAAAAAAAA==',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g success',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g invoke [2]',
  'Program log: Instruction: Query',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g consumed 2943 of 189793 compute units',
  'Program return: DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g CgAAAFVTRFQgLyBVU0Q=',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g success',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g invoke [2]',
  'Program log: Instruction: Query',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g consumed 2332 of 183111 compute units',
  'Program return: DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g CA==',
  'Program DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g success',
  'Program log: USDT / USD price is 1.00040000',
  'Program <PROGRAM_ID> consumed 21585 of 200000 compute units',
  'Program return: DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g CA==',
  'Program <PROGRAM_ID> success'
]
Success
```
