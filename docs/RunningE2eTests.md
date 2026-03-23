# Running tests

## Installation
`make build && make install`

## Configuration
Environment setup and test configuration live under `integration-tests/devenv`. Configs are composed from:

- `integration-tests/devenv/env.toml` for base environment settings
- `integration-tests/devenv/products/solana/basic.toml` for the embedded OCR2 setup
- `integration-tests/devenv/products/solana/plugins.toml` to run with plugin binaries
- `integration-tests/devenv/products/solana/soak.toml` to extend the smoke config for soak

Important: environment startup is decoupled from test logic. Tests no longer start the environment. You must run the environment setup (`go run ./cmd u ...`) before running any tests. This generates `integration-tests/devenv/env-out.toml`, which the tests read.

## Run tests locally

### Smoke (embedded)
```
cd integration-tests/devenv
go run ./cmd u env.toml,products/solana/basic.toml
cd ../tests
go test -v -timeout 30m -run TestSolanaOCRV2Smoke
```

### Smoke (plugins)
```
cd integration-tests/devenv
go run ./cmd u env.toml,products/solana/basic.toml,products/solana/plugins.toml
cd ../tests
go test -v -timeout 30m -run TestSolanaOCRV2Smoke
```

### Soak (embedded)
```
cd integration-tests/devenv
go run ./cmd u env.toml,products/solana/basic.toml,products/solana/soak.toml
go run ./cmd obs up -f
cd ../tests
go test -v -timeout 4h -run TestSolanaOCRV2Soak
```

### Soak (plugins)
```
cd integration-tests/devenv
go run ./cmd u env.toml,products/solana/basic.toml,products/solana/soak.toml,products/solana/plugins.toml
go run ./cmd obs up -f
cd ../tests
go test -v -timeout 4h -run TestSolanaOCRV2Soak
```

## GitHub workflows

### `e2e_custom_cl.yml` (PR + manual)
Workflow runs on PRs and `workflow_dispatch` with input `cl_branch_ref` (Chainlink repo branch to integrate).

What it does:
- Builds current contract artifacts.
- Builds a custom Chainlink image (or reuses an existing one if available).
- Runs smoke tests as a matrix:
  - `embedded`: `env.toml,products/solana/basic.toml`
  - `plugins`: `env.toml,products/solana/basic.toml,products/solana/plugins.toml`
- Runs program upgrade tests only when:
  - Contracts have changed, and
  - Current artifacts differ from the pinned previous release artifacts

### `soak.yml` (on-demand soak)
Navigate to the [workflow](https://github.com/smartcontractkit/chainlink-solana/actions/workflows/soak.yml). Inputs are:

- `cl_image_tag` (required): Chainlink image tag
- `cl_image_ecr` (optional): ECR repo name (default `chainlink`)

The workflow builds contract artifacts, starts the environment via `integration-tests/devenv`, then runs soak tests with the same embedded/plugins matrix.
