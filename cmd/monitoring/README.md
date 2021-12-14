# Solana Onchain monitoring


## Run locally

The monitor requires a schema registry, a kafka instance and a zookeper.
You can use the `./ops/monitoring/docker-compose.yml`

The contracts needs to be deployed and the required accounts created.
You can follow the gauntlet instructions in `smartcontractkit/chainlink-solana`.

You need to provide a description of the feeds. It can either be a file path (say /tmp/feeds.json) or an RDD url.
An example of a compatible json encoded feeds configuration is:
```json
[
  {
    "name": "sol/usd",
    "path": "sol-usd",
    "symbol": "$",
    "heartbeat": 1,
    "contract_type": "ocr2",
    "status": "live",

    "contract_address_base58": "2jVYiZgQ5disuAUMxrF1LkUyhZuqvRCrx1LfB555XUUv",
    "transmissions_account_base58": "2jVYiZgQ5disuAUMxrF1LkUyhZuqvRCrx1LfB555XUUv",
    "state_account_base58": "2jVYiZgQ5disuAUMxrF1LkUyhZuqvRCrx1LfB555XUUv",

    "poll_interval_milliseconds": 1000
  },
  {
    "name": "link/usd",
    "path": "link-usd",
    "symbol": "L",
    "heartbeat": 1,
    "contract_type": "ocr2",
    "status": "live",

    "contract_address_base58": "GUnMZPbhxkimy9ssXyPG8rVTPBPFzL24W4vFuxyEZm66",
    "transmissions_account_base58": "GUnMZPbhxkimy9ssXyPG8rVTPBPFzL24W4vFuxyEZm66",
    "state_account_base58": "GUnMZPbhxkimy9ssXyPG8rVTPBPFzL24W4vFuxyEZm66",

    "poll_interval_milliseconds": 1000
  }
]
```

To build and execute the monitor locally, run:

```bash
go run ./cmd/monitoring/*.go \
-solana.rpc_endpoint="http://127.0.0.1:8899" \
-solana.network_name="solana-devnet" \
-solana.network_id="solana-devnet" \
-solana.chain_id="1" \
-kafka.topic="solana-devnet" \
-kafka.brokers="localhost:29092" \
-kafka.client_id="solana" \
-kafka.security_protocol="PLAINTEXT" \
-kafka.sasl_mechanism="PLAIN" \
-kafka.sasl_username="" \
-kafka.sasl_password="" \
-schema_registry.url="http://localhost:8989" \
-schema_registry.username="" \
-schema_registry.password="" \
-feeds.file_path="/tmp/feeds.json" \
-http.address="localhost:3000"
```

See `go run ./cmd/monitoring/*.go -help` for details.

To generate random data instead of reading from the chain, use the env var `TEST_MODE=enabled`.

## Build docker image

```bash
docker build -f ./ops/monitoring/Dockerfile -t solana-onchain-monitor:0.1.0 .
```
