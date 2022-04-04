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
  }
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
  }
]
```

To build and execute the monitor locally, run:

```bash
SOLANA_RPC_ENDPOINT="http://127.0.0.1:8899" \
SOLANA_NETWORK_NAME="solana-devnet" \
SOLANA_NETWORK_ID="solana-devnet" \
SOLANA_CHAIN_ID="1" \
SOLANA_READ_TIMEOUT="2s" \
SOLANA_POLL_INTERVAL="5s" \
KAFKA_BROKERS="localhost:29092" \
KAFKA_CLIENT_ID="solana" \
KAFKA_SECURITY_PROTOCOL="PLAINTEXT" \
KAFKA_SASL_MECHANISM="PLAIN" \
KAFKA_SASL_USERNAME="" \
KAFKA_SASL_PASSWORD="" \
KAFKA_CONFIG_SET_SIMPLIFIED_TOPIC="config_set_simplified" \
KAFKA_TRANSMISSION_TOPIC="transmission_topic" \
SCHEMA_REGISTRY_URL="http://localhost:8989" \
SCHEMA_REGISTRY_USERNAME="" \
SCHEMA_REGISTRY_PASSWORD="" \
HTTP_ADDRESS="localhost:3000" \
FEEDS_URL="http://localhost:4000" \
go run ./cmd/monitoring/main.go --run-mode=monitor
```

See `go run ./cmd/monitoring/*.go -help` for details.

To generate random data instead of reading from the chain, use the env var `TEST_MODE=enabled`.

## Build docker image

```bash
docker build -f ./ops/monitoring/Dockerfile -t solana-onchain-monitor:0.1.0 .
```

# Solana Ingestor

## Running

```bash
SOLANA_WS_ENDPOINT="wss:127.0.0.1:8899" \
SOLANA_NETWORK_NAME="solana-devnet" \
SOLANA_NETWORK_ID="solana-devnet" \
SOLANA_CHAIN_ID="1" \
SOLANA_STATE_KAFKA_TOPIC="state" \
SOLANA_TRANSMISSIONS_KAFKA_TOPIC="transmissions" \
SOLANA_EVENTS_KAFKA_TOPIC="events" \
KAFKA_BROKERS="localhost:29092" \
KAFKA_CLIENT_ID="solana" \
KAFKA_SECURITY_PROTOCOL="PLAINTEXT" \
KAFKA_SASL_MECHANISM="PLAIN" \
KAFKA_SASL_USERNAME="" \
KAFKA_SASL_PASSWORD="" \
KAFKA_CONFIG_SET_SIMPLIFIED_TOPIC="config_set_simplified" \
KAFKA_TRANSMISSION_TOPIC="transmission_topic" \
SCHEMA_REGISTRY_URL="http://localhost:8989" \
SCHEMA_REGISTRY_USERNAME="" \
SCHEMA_REGISTRY_PASSWORD="" \
HTTP_ADDRESS="localhost:3000" \
FEEDS_URL="http://localhost:4000" \
go run ./cmd/monitoring/main.go --run-mode=ingesto
```

## List of events
- These events are only only issued by the ocr2 contract.

```
SetConfig {
    pub config_digest: [u8; 32],
    pub f: u8,
    pub signers: Vec<[u8; 20]>,
}

SetBilling {
    pub observation_payment_gjuels: u32,
    pub transmission_payment_gjuels: u32,
}

RoundRequested {
    pub config_digest: [u8; 32],
    pub requester: Pubkey,
    pub epoch: u32,
    pub round: u8,
}

NewTransmission {
    pub round_id: u32,
    pub config_digest: [u8; 32],
    pub answer: i128,
    pub transmitter: u8,
    pub observations_timestamp: u32,
    pub observer_count: u8,
    pub observers: [u8; 19], // Can't use MAX_ORACLES because of IDL parsing issues
    pub juels_per_lamport: u64,
    pub reimbursement_gjuels: u64,
}
```
