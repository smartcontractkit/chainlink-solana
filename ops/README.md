# Local Testing Environment

[WIP - work in progress]

Using `pulumi` spin up a local testing environment using docker containers.
- Deploy the necessary relay components (relay, CL node, psql DB, price feed adapters)
- Connect the components together
```
Blockchain <-> Relay <-> CL node <-> price adapters
                |<-> DB <->|
```

- Deploy and configure the necessary contracts (LINK token, aggregator contract)
- Create the expected job specs for reporting to the aggregator contract

## Dependencies
- [Pulumi](#pulumi-installation-instruction)
- [Solana Test Validator](#local-solana-testnet)

## Usage
Set a `.env` file inside of `chainlink-solana/gauntlet`. The private key can be generated the same way as the program accounts (`solana-keygen`) then copied into the env file.
```
# .env
PRIVATE_KEY=[96,13,...]
SECRET=some random local testing secret
```

Generate keypairs for program accounts (usually only need to be done once)
```bash
solana-keygen new -o <path>

# examples (from root)
solana-keygen new -o ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/access_controller.json
solana-keygen new -o ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/deviation_flagging_validator.json
solana-keygen new -o ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/ocr2.json
```

Add the pubkeys to the their respective program (do for each program)
```rust
// example in programs/ocr2/src/lib.rs
declare_id!("CF6b2XF6BZw65aznGzXwzF5A8iGhDBoeNYQiXyH4MWdQ");
```

Compile program artifacts (do each time contract changes)
```bash
# from root
./tools/shell
cd contracts
anchor build

# then exit environment
cp contracts/target/deploy/access_controller.so gauntlet/packages/gauntlet-solana-contracts/artifacts/bin/access_controller.so
cp contracts/target/deploy/deviation_flagging_validator.so gauntlet/packages/gauntlet-solana-contracts/artifacts/bin/deviation_flagging_validator.so
cp contracts/target/deploy/ocr2.so gauntlet/packages/gauntlet-solana-contracts/artifacts/bin/ocr2.so
```

Start up the solana test validator (recommend always using `-r` for a clean slate, runs into deployment issues otherwise)
```bash
# start validator
solana-test-validator -r

# in another terminal airdrop funds to your gauntlet deployer account (see below if need to configure CLI for local validator)
solana airdrop 100 2CbCTf2V95kMfNA31yYaqJ9oVX7MN71RU6zvvg27PgSz
```

Start up the pulumi environment (tweak the `Pulumi.localnet.yaml` file if necessary)
```bash
# start up the environment
pulumi up -y -s localnet

# destroy the environment
pulumi destroy -y -s localnet
```

## Local Solana Testnet
Documentation:
* https://docs.solana.com/developing/test-validator
* https://docs.solana.com/developing/clients/jsonrpc-api

```bash
# start up local test validator
solana-test-validator
solana-test-validator -r # reset network state

# configure solana CLI
solana config set --url http://127.0.0.1:8899

# airdrop tokens to deployer + node addresses
solana airdrop 10 <account>

# monitor chain logs
solana logs
```

## Pulumi Installation Instruction
Infrastructure management tool.

```bash
# create stack for a new network
pulumi stack init <network>

# select network/stack to use
pulumi stack select

# start stack
pulumi up

# stop stack and remove artifacts
pulumi destroy

# remove all traces of stack (usually not needed)
pulumi stack rm <network>
```

Notes:
* Installation: highly recommend using [`asdf`](https://asdf-vm.com/) for version management
   ```
   asdf plugin add pulumi
   asdf install pulumi latest
   asdf global pulumi latest
   ```
* May require setting environment variable `export PULUMI_CONFIG_PASSPHRASE=` (no need to set it to anything unless you want to)
* [Using Pulumi without pulumi.com](https://www.pulumi.com/docs/troubleshooting/faq/#can-i-use-pulumi-without-depending-on-pulumicom): tl;dr - `pulumi login --local`
