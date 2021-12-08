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

## Prep
```
solana-keygen new -o ./packages-ts/gauntlet-solana-contracts/artifacts/programId/*.json
```

## Usage
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
