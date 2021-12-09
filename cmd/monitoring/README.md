# Solana Onchain monitoring

## Help

```
go run ./cmd/monitoring/*.go -help
```

## Run locally

Requirements:
- schema registry
- kafka
- solana validator
- contracts deployed and accounts created

- feeds description in either a file (say /tmp/feeds.json) or an RDD url

Fetch latest master and execute
```
go run ./cmd/monitoring/*.go \
-solana.rpc_endpoint="http://127.0.0.1:8899" \
-solana.network_name="solana-devnet" \
-solana.network_id="solana-devnet" \
-solana.chain_id="1" \

-kafka.topic="solana-devnet" \
-kafka.brokers="localhost:29029" \
-kafka.client_id="monitoring" \
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

## Build docker image

```
docker build -f monitoring.Dockerfile -t solana-onchain-monitor:0.1.0 .
```
