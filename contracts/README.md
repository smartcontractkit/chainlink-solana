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

Make sure to run `pnpm i` to fetch mocha and other test dependencies.

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

### Using nix shell to test locally

Make sure you have run this in the contracts/ folder

```
solana-keygen new -o id.json
```

1. In `shell.nix` comment out:

```
# (rust-bin.stable.latest.default.override { extensions = ["rust-src"]; })
# lld_11
```

to use local version of `rustup` and `cargo`

2. Ensure `ts/` is built with `/ts && pnpm build`

3. As of 05/21/2025 works with:

- `cargo 1.79.0 (ffa9cf99a 2024-06-03)`
- `rustc 1.79.0 (129f3b996 2024-06-10)`

4. `anchor build && anchor test`

### `anchor-go` bindings generation

Install `https://github.com/gagliardetto/anchor-go`

Current version: [v0.2.3](https://github.com/gagliardetto/anchor-go/tree/v0.2.3)

```bash
./scripts/anchor-go-gen.sh
```
