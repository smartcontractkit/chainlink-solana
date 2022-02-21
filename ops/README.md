# Local Environment

[WIP - work in progress]

- [Local Environment](#local-environment)
  - [Usage](#usage)
  - [Local Solana Testnet](#local-solana-testnet)
  - [Pulumi Installation Instruction](#pulumi-installation-instruction)

Using `pulumi` spin up a local testing environment using docker containers:

- Deploy the necessary relay components (CL node, psql DB, price feed adapters)
- Connect the components together
- Deploy and configure the necessary contracts (LINK token, aggregator contract)
- Create the expected job specs for reporting to the aggregator contract

## Usage

Generate the key used for deployments (Gauntlet). The key can be generated using `solana-keygen` then copied into the env file.

```bash
solana-keygen new -o <path>
```

Add a `./gauntlet/.env` file`:

```
# .env
PRIVATE_KEY=[96,13,...]
SECRET=some random local testing secret
```

From the root of the repo, use the following commands to use localnet keys in the repo to build artifacts and add them to gauntlet:

```bash
# start up the anchor shell
./scripts/anchor-shell.sh

# build artifacts and copy
./scripts/setup-local-artifacts.sh
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

- https://docs.solana.com/developing/test-validator
- https://docs.solana.com/developing/clients/jsonrpc-api

Note:
`Program failed to complete: ELF error: Unresolved symbol (sol_secp256k1_recover) at instruction #53009 (ELF file offset 0x677a0)`

- Resolved by updating to the latest solana CLI `solana-install update`

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

- Installation: highly recommend using [`asdf`](https://asdf-vm.com/) for version management
  ```
  asdf plugin add pulumi
  asdf install pulumi latest
  asdf global pulumi latest
  ```
- May require setting environment variable `export PULUMI_CONFIG_PASSPHRASE=` (no need to set it to anything unless you want to)
- [Using Pulumi without pulumi.com](https://www.pulumi.com/docs/troubleshooting/faq/#can-i-use-pulumi-without-depending-on-pulumicom): tl;dr - `pulumi login --local`
