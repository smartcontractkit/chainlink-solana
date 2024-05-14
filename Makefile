BIN_DIR = bin
export GOPATH ?= $(shell go env GOPATH)
export GO111MODULE ?= on
export PROJECT_SERUM_VERSION ?=v0.25.0
export PROJECT_SERUM_IMAGE ?= projectserum/build:$(PROJECT_SERUM_VERSION)

LINUX=LINUX
OSX=OSX
WINDOWS=WIN32
OSFLAG :=
ifeq ($(OS),Windows_NT)
	OSFLAG = $(WINDOWS)
else
	UNAME_S := $(shell uname -s)
	ifeq ($(UNAME_S),Linux)
		OSFLAG = $(LINUX)
	endif
	ifeq ($(UNAME_S),Darwin)
		OSFLAG = $(OSX)
	endif
endif

download:
	go mod download

install:
ifeq ($(OSFLAG),$(WINDOWS))
	echo "If you are running windows and know how to install what is needed, please contribute by adding it here!"
	exit 1
endif
ifeq ($(OSFLAG),$(OSX))
	brew install asdf
	asdf plugin-add nodejs || true
	asdf plugin-add rust || true
	asdf plugin-add golang || true
	asdf plugin-add ginkgo || true
	asdf plugin-add pulumi || true
	asdf plugin add actionlint || true
	asdf plugin add shellcheck || true
	asdf plugin add kubectl || true
	asdf install
endif
ifeq ($(OSFLAG),$(LINUX))
ifneq ($(CI),true)
	# install nix
	sh <(curl -L https://nixos-nix-install-tests.cachix.org/serve/vij683ly7sl95nnhb67bdjjfabclr85m/install) --daemon --tarball-url-prefix https://nixos-nix-install-tests.cachix.org/serve --nix-extra-conf-file ./nix.conf
endif
	go install github.com/onsi/ginkgo/v2/ginkgo@v$(shell cat ./.tool-versions | grep ginkgo | sed -En "s/ginkgo.(.*)/\1/p")
endif

.PHONY: projectserum_version
projectserum_version:
	@echo "${PROJECT_SERUM_VERSION}"

anchor_shell:
	docker run --rm -it -v $(shell pwd):/workdir --entrypoint bash ${PROJECT_SERUM_IMAGE}

build_js:
	cd gauntlet && yarn install --frozen-lockfile && yarn bundle

build_contracts:
	docker run --rm -it -v $(shell pwd):/workdir ${PROJECT_SERUM_IMAGE} /bin/bash ./scripts/anchor-build.sh

build_contracts_local:
	docker run --rm -it -v $(shell pwd):/workdir ${PROJECT_SERUM_IMAGE} /bin/bash ./scripts/setup-local.sh

build_contracts_staging:
	docker run --rm -it -v $(shell pwd):/workdir ${PROJECT_SERUM_IMAGE} /bin/bash ./scripts/setup-staging.sh

cp_gauntlet_idl:
	cp ./contracts/target/idl/*.json ./gauntlet/packages/gauntlet-solana-contracts/artifacts/schemas

build: build_js build_contracts cp_gauntlet_idl

build_local: build_js build_contracts_local cp_gauntlet_idl

build_staging: build_js build_contracts_staging cp_gauntlet_idl

test_relay_unit:
	go build -v ./pkg/...
	go test -v ./pkg/...

test_smoke:
	cd ./integration-tests &&\
	SELECTED_NETWORKS=SIMULATED go test -timeout 24h -count=1 -json $(args) -run TestSolanaOCRV2Smoke ./smoke 2>&1 | tee /tmp/gotest.log | gotestfmt

test_ocr_soak:
	cd ./integration-tests &&\
	SELECTED_NETWORKS=SIMULATED go test -timeout 24h -count=1 -json $(args) ./soak 2>&1 | tee /tmp/gotest.log | gotestfmt

gomodtidy:
	go mod tidy
	cd ./integration-tests && go mod tidy

.PHONY: lint-go-integration-tests
lint-go-integration-tests:
	cd ./integration-tests && golangci-lint --max-issues-per-linter 0 --max-same-issues 0 --color=always --exclude=dot-imports --timeout 10m --out-format checkstyle:golangci-lint-integration-tests-report.xml run || true

.PHONY: lint-go-relay
lint-go-relay:
	cd ./pkg && golangci-lint --max-issues-per-linter 0 --max-same-issues 0 --color=always --exclude=dot-imports --timeout 10m --out-format checkstyle:golangci-lint-relay-report.xml run || true

.PHONY: upgrade-e2e-solana-image
upgrade-e2e-solana-image:
	./scripts/update-solana.sh

.PHONY: update-e2e-core-deps
upgrade-e2e-core-deps:
	cd ./integration-tests && ../scripts/update-e2e.sh
