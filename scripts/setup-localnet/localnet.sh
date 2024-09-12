#!/usr/bin/env bash

set -euo pipefail
cpu_struct="linux";

# Clean up first
bash "$(dirname -- "$0";)/localnet.down.sh"

container_version=v2.0.8
container_name="chainlink-solana.test-validator"

echo "Starting $container_name@$container_version"

# NOTE: solanalabs/solana docker image only supports linux/amd64
# https://hub.docker.com/r/solanalabs/solana/tags
# If you are running on an ARM machine, Following error will be thrown:
# "Incompatible CPU detected: missing AVX support. Please build from source on the target "
docker run -d \
  --platform linux/amd64 \
  -p 127.0.0.1:8899:8899 \
  -p 127.0.0.1:8900:8900 \
  -p 127.0.0.1:9900:9900 \
  --name "${container_name}" \
  --entrypoint /bin/sh \
  "anzaxyz/agave:${container_version}" \
  -c "solana-test-validator && echo 'Validator started successfully'"
  # 	--network-alias "${container_name}" \
  # 	--network chainlink \

echo "Waiting for test validator to become ready.."
start_time=$(date +%s)
prev_output=""
while true
do
  output=$(docker logs chainlink-solana.test-validator 2>&1)
  if [[ "${output}" != "${prev_output}" ]]; then
    echo -n "${output#$prev_output}"
    prev_output="${output}"
  fi

  if [[ $output == *"Finalized Slot"* ]]; then
    echo ""
    echo "solana-test-validator is ready."
    exit 0
  fi

  if [[ $output == *"Incompatible CPU detected"* || $output == *"Aborted"* ]]; then
    echo ""
    echo "solanalabs/solana docker image only supports linux/amd64"
    exit 1
  fi

  current_time=$(date +%s)
  elapsed_time=$((current_time - start_time))

  if (( elapsed_time > 10 )); then
    echo "Error: Command did not become ready within 10 seconds"
    exit 1
  fi

  sleep 3
done
