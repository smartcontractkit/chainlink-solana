#!/bin/bash
set -e

cliVersion=$(grep -oh "release.solana.com/v[0-9]*.[0-9]*.[0-9]*" scripts/install-solana-ci.sh)
echo "Current Test CLI Version: $cliVersion"

cd integration-tests
testVersion=$(grep -oh "solanalabs/solana:v[0-9]*.[0-9]*.[0-9]*" testconfig/default.toml) 
echo "Current E2E Test Version: $testVersion"
cd ..

localnetVersion=$(grep -oh "container_version=v[0-9]*.[0-9]*.[0-9]*" scripts/setup-localnet/localnet.sh)
echo "Current Version in Localnet Container: $localnetVersion"

nixVersion=$(grep -oh "version = \"v[0-9]*.[0-9]*.[0-9]*\"" solana.nix)
echo "Current Version in Nix packages: $nixVersion"

latestTag=$(curl https://api.github.com/repos/solana-labs/solana/releases/latest | jq -r '.tag_name')
latestVersion="solanalabs/solana:$latestTag"
latestCLI="release.solana.com/$latestTag"
latestLocalnet="container_version=$latestTag"
latestNix="version = \"$latestTag\""
echo "Latest Solana Mainnet Version: $latestTag"

if [ "$testVersion" = "$latestVersion" ] && [ "$cliVersion" = "$latestCLI" ] ; then
  echo "Solana Versions Are Up To Date"
  exit 0
fi

echo "Replacing Solana Image Version"

if [ "$(uname -s)" = "Darwin" ]; then
  sed -i '' -e "s~$cliVersion~$latestCLI~" scripts/install-solana-ci.sh
  cd integration-tests
  sed -i '' -e "s~$testVersion~$latestVersion~" testconfig/default.toml
  cd ..
  sed -i '' -e "s~$localnetVersion~$latestLocalnet~" scripts/setup-localnet/localnet.sh
  sed -i '' -e "s~$nixVersion~$latestNix~" solana.nix
else
  sed -i -e "s~$cliVersion~$latestCLI~" scripts/install-solana-ci.sh
  cd integration-tests
  sed -i -e "s~$testVersion~$latestVersion~" testconfig/default.toml
  cd ..
  sed -i -e "s~$containerVersion~$latestContainer~" scripts/setup-localnet/localnet.sh
  sed -i -e "s~$nixVersion~$latestNix~" solana.nix
fi
cd ..

echo "Done"
