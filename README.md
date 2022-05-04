# Chainlink Solana

## Quick Start

For more information, see the [Chainlink Solana Documentation](./docs/).

## Local M1/ARM64 build

In order to build the project locally for arm64, one might find it useful to link the dependency libraries dynamically.
The following steps install arm64 version of `librdkafka` and link it dynamically during the build:

1. Install the dependencies
    ```sh 
    brew install openssl pkg-config librdkafka
    ```
2. Follow the after-install instructions to let `pkg-config` find `openssl`
    ```sh
    export PKG_CONFIG_PATH="/opt/homebrew/opt/openssl@3/lib/pkgconfig"
    ```
3. Build using dynamic tag
    ```sh
    go build --tags dynamic ./...
    ```
