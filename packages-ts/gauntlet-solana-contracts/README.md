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
- Deviation flagging validator: `deviation_flagging_validator`

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


