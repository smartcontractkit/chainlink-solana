# Ingestor

The ingestor is a different "running mode" for the Solana on-chain monitor.
It consumes data from the Solana chain and publishes a representation of this data to a kafka cluster.

The ingestor runs the following pipline:
- a pipline that subscribes to all the blocks that have transactions for the aggregator contract.
- a pipline that subscribes to all the logs emitted by the aggregator contract.
- a pipline that subscribes to all the state and transmission account changes for each feed deployed

The high-level structure of a pipeline is:

```
          +-------------------------pipeline------------------------------+
(solana)--|-->(updater)-->(decoder)-->(mapper)-->(encoder)-->(publisher)--|-->(kafka)
          +---------------------------------------------------------------+
```

## Running

```bash
SOLANA_RUN_MODE=ingestor \
SOLANA_WS_ENDPOINT="wss://127.0.0.1:9988/ws" \
SOLANA_NETWORK_NAME="solana-mainnet" \
SOLANA_NETWORK_ID="solana-mainnet" \
SOLANA_CHAIN_ID="1" \
SOLANA_STATES_KAFKA_TOPIC="states" \
SOLANA_TRANSMISSIONS_KAFKA_TOPIC="transmissions" \
SOLANA_EVENTS_KAFKA_TOPIC="events" \
SOLANA_BLOCKS_KAFKA_TOPIC="blocks" \
KAFKA_BROKERS="localhost:29092" \
KAFKA_CLIENT_ID="som-ingestor" \
KAFKA_SECURITY_PROTOCOL="PLAINTEXT" \
KAFKA_SASL_MECHANISM="PLAIN" \
KAFKA_SASL_USERNAME="" \
KAFKA_SASL_PASSWORD="" \
SCHEMA_REGISTRY_URL="http://localhost:8989" \
SCHEMA_REGISTRY_USERNAME="" \
SCHEMA_REGISTRY_PASSWORD="" \
HTTP_ADDRESS="localhost:3000" \
FEEDS_URL="http://localhost:4000/feeds.json" \
NODES_URL="http://localhost:4000/nodes.json" \
go run ./cmd/monitoring/main.go
```

## List of events published on the SOLANA_EVENTS_KAFKA_TOPIC
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

