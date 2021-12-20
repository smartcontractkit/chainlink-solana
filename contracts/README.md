# Chainlink Solana contracts (programs)

## Build

To build on the host:

```
anchor build
```

To build inside a docker environment:

```bash
anchor build --verifiable
```

To build for a specific network, specify via a cargo feature:

```bash
anchor build -- --features mainnet
```

Available networks with declared IDs:

- mainnet
- testnet
- devnet
- localnet (default)

## Test

Make sure to run `yarn install` to fetch mocha and other test dependencies.

Start a dockerized shell that contains Solana and Anchor:

```bash
./scripts/anchor-shell.sh
```

Next, generate a keypair for anchor:

```bash
solana-keygen new -o id.json
```

Run anchor tests (automatically tests against a local node).

```bash
anchor test
```

### `anchor-go` bindings generation

Install `https://github.com/gagliardetto/anchor-go`

```bash
./scripts/anchor-go-gen.sh
```
