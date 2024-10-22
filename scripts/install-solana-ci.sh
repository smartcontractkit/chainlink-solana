#!/usr/bin/env bash

set -euxo pipefail

sh -c "$(curl -sSfL https://release.anza.xyz/v1.18.26/install)"
echo "PATH=$HOME/.local/share/solana/install/active_release/bin:$PATH" >> $GITHUB_ENV
