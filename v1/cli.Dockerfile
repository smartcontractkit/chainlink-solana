FROM rust:1.54.0 AS build

# Solana RPC client links against these
RUN apt-get update && apt-get install -y libhidapi-dev libudev-dev

WORKDIR /usr/src/chainlink-solana
# XXX: You need a new for each member in the workspace for this trick to work
RUN USER=root cargo new cli
RUN USER=root cargo new program
COPY Cargo.toml Cargo.lock .
RUN cargo build -p cli --release

COPY program/Cargo.toml program/
COPY program/src program/src
COPY cli/Cargo.toml cli/
COPY cli/src cli/src
# NOTE: Cannot use x86_64-unknown-linux-musl because of `ring` dependency
RUN cargo build --bin cli --release

FROM debian:buster-slim
RUN apt-get update && apt-get install -y libssl1.1 ca-certificates
COPY --from=build /usr/src/chainlink-solana/target/release/cli .
USER 1000
ENTRYPOINT ["./cli"]
