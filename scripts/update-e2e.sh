#!/bin/bash
set -e

# get current develop branch sha
SHA=$(curl https://api.github.com/repos/smartcontractkit/chainlink/commits/develop | jq -r '.sha')
echo "Chainlink Develop Commit: $SHA"

# update dependencies
go get github.com/smartcontractkit/chainlink/integration-tests@$SHA
go mod tidy || echo -e "------\nInitial go mod tidy failed - will update chainlink dep and try tidy again\n------"
go get github.com/smartcontractkit/chainlink/v2@$SHA
go mod tidy
