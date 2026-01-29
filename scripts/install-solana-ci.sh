#!/usr/bin/env bash

set -euxo pipefail

sh -c "$(curl -sSfL https://release.anza.xyz/v3.1.8/install)"
echo "PATH=$HOME/.local/share/solana/install/active_release/bin:$PATH" >> $GITHUB_ENV
