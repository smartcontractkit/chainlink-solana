# chainlink-solana

## Build

To build on the host:

```
anchor build
```


To build inside a docker environment:

```
anchor build --verifiable
```

## Test

Make sure to run `yarn install` to fetch mocha and other test dependencies.

Start a dockerized shell that contains Solana and Anchor:

```
tools/shell
```

Next, generate a keypair for anchor:

```
solana-keygen new -o id.json
```

Run anchor tests (automatically tests against a local node).

```
anchor test
```

### `anchor-go` bindings generation
Install `https://github.com/gagliardetto/anchor-go`
```
./anchor-generate.sh
```
