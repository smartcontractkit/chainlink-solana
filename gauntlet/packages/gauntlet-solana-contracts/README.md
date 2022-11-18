# gauntlet-solana

### Prepare

```
yarn
```

Create a `.env` on the project root directory, and set your private key as:

```
# .env
PRIVATE_KEY=[38,56,112,28,28,122, ...]
```

Optional: Build binary

```
yarn bundle
```

## Execute commands:

### Contracts available

- Access Controller: `access_controller`
- OCR2: `ocr2`
- Flags: `flags`,
- Deviation flagging store: `store`

### Commands

To execute with binary, change `yarn gauntlet` for `./bin/chainlink-solana-[macos|linux] <command>`

- Deploy

Deployment is available for any of the contracts in the list

```
yarn gauntlet <contract_name>:deploy --network=<testnet|devnet>
```

- Commands available

Contract functions are only available in some contracts. Get the latest supported commands running

```
yarn gauntlet help
```

- Interact with contract

```
yarn gauntlet <contract_name>:<contract_function> --network=<testnet|devnet> --state=<state_account_public_key> [OPTIONAL: --<function_parameter_name=<value>] <contract_address>
```

Command example:

```
yarn gauntlet <contract_name>:<contract_function> --help
```

## Testing Locally

```
scripts/localnet.sh
```

Starts a `solana-test-validator` with the programs pre-loaded and airdrops funds to the default PRIVATE_KEY used by `.env.local`.

### Running

A flow command can be executed to set up everything needed. Program deployments, initialization, configuration setting and single transmission.

```
SKIP_PROMPTS=true yarn gauntlet ocr2:setup:flow --network=local
```

The result of the command will be stored on `/flow-report.json` file

If any error occurs, the flow can be started from that point, using the previous flow report, as:

```
SKIP_PROMPTS=true yarn gauntlet ocr2:setup:flow --network=local --withReport --start=<step number>
```

After the flow has finished succesfully, more transmissions can be executed, either by:

```
SKIP_PROMPTS=true yarn gauntlet ocr2:setup:flow --network=local --withReport --start=12 --round=2
```

or

```
yarn gauntlet ocr2:transmit --network=local --state=<state_account_public_key> --transmissions=<state_account_public_key> --store=<state_account_public_key> --accessController=<state_account_public_key> --round=<round number>
```

The account public keys can be found inside the `/flow-report.json` file

Make sure to increment the round after every transmission
