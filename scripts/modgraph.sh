#!/usr/bin/env bash

# Generates go.md

set -e

echo "# smartcontractkit Go modules
## Main module
\`\`\`mermaid
flowchart LR
"
go mod graph | modgraph -prefix github.com/smartcontractkit/
echo "\`\`\`"

echo "## All modules
\`\`\`mermaid
flowchart LR
"
gomods graph | modgraph -prefix github.com/smartcontractkit/
echo "\`\`\`"