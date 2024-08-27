FROM debian:bullseye as base
RUN rm /bin/sh && ln -s /bin/bash /bin/sh
WORKDIR /workspace

RUN mkdir -pv "/workspace/bin"

ENV PATH="/workspace/bin:${PATH}"

FROM base as builder

# Install os deps
RUN apt update && \
  apt-get install -y build-essential clang cmake curl libudev-dev pkg-config protobuf-compiler && \
  rm -rf /var/lib/apt/lists/*

# Setup rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- --default-toolchain 1.75.0 -y
ENV PATH="/root/.cargo/bin:${PATH}"

# Set the solana version
# Example usage: docker build --build-arg SOLANA_VERSION=$(./get-latest-validator-release-version.sh) -t solana-test-validator .
# or, for specific version: docker build --build-arg SOLANA_VERSION=1.18.22 -t solana-test-validator .
ARG SOLANA_VERSION=none

# Check if the ARG is provided
RUN if [ "$SOLANA_VERSION" = "none" ]; then \
  echo "Error: SOLANA_VERSION argument is not provided!" >&2; \
  exit 1; \
  else \
  echo "Building with Solana version: $SOLANA_VERSION"; \
  fi

# Get the solana source
RUN curl https://codeload.github.com/solana-labs/solana/tar.gz/refs/tags/v$SOLANA_VERSION | tar xvz
RUN mv /workspace/solana-$SOLANA_VERSION /workspace/solana

# Build the solana-test-validator
WORKDIR /workspace/solana
RUN cargo build --bin solana-test-validator --release
RUN cp target/release/solana-test-validator /workspace/bin/

FROM base as final

## Install os deps
RUN apt update && \
  apt-get install -y bzip2 curl && \
  rm -rf /var/lib/apt/lists/*

COPY --from=builder /workspace/bin/* /workspace/bin