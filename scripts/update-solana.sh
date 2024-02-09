#!/bin/bash
set -e

testVersion=$(grep -oh "solanalabs/solana:v[0-9]*.[0-9]*.[0-9]*" */**/*.go) 
echo "Current E2E Test Version: $testVersion"

latestTag=$(curl https://api.github.com/repos/solana-labs/solana/releases/latest | jq -r '.tag_name')
latestVersion="solanalabs/solana:$latestTag"
echo "Latest Solana Mainnet Version: $latestVersion"

if [ "$testVersion" = "$latestVersion" ]; then
  echo "E2E Tests Are Up To Date"
  exit 0
fi

echo "Replacing Solana Image Version"

if [ "$(uname -s)" = "Darwin" ]; then
  sed -i '' -e "s~$testVersion~$latestVersion~" */**/*.go
else
  sed -i -e "s~$testVersion~$latestVersion~" */**/*.go
fi

echo "Done"
export SOL_IMAGE="$latestVersion"
