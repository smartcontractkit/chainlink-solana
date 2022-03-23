BIN_DIR = bin
export GOPATH ?= $(shell go env GOPATH)
export GO111MODULE ?= on
export PROJECT_SERUM_IMAGE ?= projectserum/build:v0.22.1

download:
	go mod download

install:
	go get github.com/onsi/ginkgo/v2/ginkgo/generators@v2.1.2
	go get github.com/onsi/ginkgo/v2/ginkgo/internal@v2.1.2
	go get github.com/onsi/ginkgo/v2/ginkgo/labels@v2.1.2
	go install github.com/onsi/ginkgo/v2/ginkgo

anchor_shell:
	docker run --rm -it -v $(shell pwd):/workdir --entrypoint bash ${PROJECT_SERUM_IMAGE}

build_js:
	cd gauntlet && yarn install --frozen-lockfile && yarn bundle

build_contracts:
	docker run --rm -it -v $(shell pwd):/workdir ${PROJECT_SERUM_IMAGE} /bin/bash ./scripts/anchor-build.sh

build: build_js build_contracts

test_relay_unit:
	go build -v ./pkg/solana/...
	go test -v ./pkg/solana/...

test_smoke:
	SELECTED_NETWORKS=solana NETWORK_SETTINGS=$(shell pwd)/tests/e2e/networks.yaml ginkgo tests/e2e/smoke

test_chaos:
	SELECTED_NETWORKS=solana NETWORK_SETTINGS=$(shell pwd)/tests/e2e/networks.yaml ginkgo tests/e2e/chaos
