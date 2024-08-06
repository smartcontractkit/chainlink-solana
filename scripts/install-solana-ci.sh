#!/usr/bin/env bash

set -euxo pipefail

sh -c "$(curl -sSfL https://release.solana.com/v1.18.21/install)"
echo "PATH=$HOME/.local/share/solana/install/active_release/bin:$PATH" >> $GITHUB_ENV
