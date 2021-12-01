# Solana Integration

Repository containing the program and tooling associated with the v5 streaming aggregator deployed on Solana.

## CLI

The `cli` folder contains the interfaces and commands for interacting with the program and associated accounts.

### Feeds

```bash
# initialize new feed
cargo run -- feed init --fee-payer <fee-payer> --owner <owner>

# configure a feed
cargo run -- feed configure --fee-payer <fee-payer> --owner <owner> --aggregator <aggregator> [oracles]...

# get configured feeds
cargo run -- feed get
```
Note: `fee-payer` and `owner` are filepaths to the keypair file.

### Adapter/Server

```bash
# start adapter
cargo run -- serve <json-rpc-url> <program-id> <oracle-keypair> <fee-payer-keypair>
```
Note: Parameters (except for the URL) are `base58` encoded.

Sample POST data
```json
{
    "value": 1, // number to report on chain
    "aggregator": "b2AQLngsWWmQbxRzi1nRCZp5V4nhvxkP5f42v1NQtyn" // aggregator account (base58)
}
```

### Utilities

```bash
# encode keypair to base58
cargo run -- encode58 --keypair '[000,000,...]'
```

## Solana

The program model on Solana enables multiple accounts to be managed within a single deployed program. In other words, a program can be deployed once but connected to multiple accounts that represent individual feeds.

### [Faucet](https://docs.solana.com/cli/usage)
Quick reference:
```bash
# airdrop (10 sol max)
solana airdrop <amount> <base56-pubkey>

# example
solana airdrop 10 4MrGqmUVBKW3WddH4cot3tUXUzF7LU8HE9yHvALbAtB6
```

### [Generating Keypairs](https://docs.solana.com/wallet-guide/file-system-wallet)
Quick reference:
```bash
# generate
solana-keygen new --outfile ~/my-solana-wallet/my-keypair.json

# get public key
solana-keygen pubkey ~/my-solana-wallet/my-keypair.json
```

### [Deploying A Program On Solana]((https://docs.solana.com/cli/deploy-a-program))
Note: this is typically not needed as the program has already been deployed.

Quick reference:
```bash
# connect to devnet
solana config set --url https://api.devnet.solana.com

# deploying
solana program deploy <PROGRAM_FILEPATH>
```
