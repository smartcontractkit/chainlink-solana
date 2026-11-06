#!/usr/bin/env bash

set -euxo pipefail
SOLANA_VERSION="${SOLANA_VERSION:-v2.1.21}"
sh -c "$(curl -sSfL https://release.anza.xyz/${SOLANA_VERSION}/install)"
echo "PATH=$HOME/.local/share/solana/install/active_release/bin:$PATH" >> $GITHUB_ENV
