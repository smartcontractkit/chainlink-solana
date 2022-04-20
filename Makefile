BIN_DIR = bin
export GOPATH ?= $(shell go env GOPATH)
export GO111MODULE ?= on
export PROJECT_SERUM_IMAGE ?= projectserum/build:v0.22.1

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
	asdf install
endif
ifeq ($(OSFLAG),$(LINUX))
ifneq ($(CI),true)
	# install nix
	sh <(curl -L https://nixos-nix-install-tests.cachix.org/serve/vij683ly7sl95nnhb67bdjjfabclr85m/install) --daemon --tarball-url-prefix https://nixos-nix-install-tests.cachix.org/serve --nix-extra-conf-file ./nix.conf
endif
	go install github.com/onsi/ginkgo/v2/ginkgo@v$(shell cat ./.tool-versions | grep ginkgo | sed -En "s/ginkgo.(.*)/\1/p")
endif

anchor_shell:
	docker run --rm -it -v $(shell pwd):/workdir --entrypoint bash ${PROJECT_SERUM_IMAGE}

build_js:
	cd gauntlet && yarn install --frozen-lockfile && yarn bundle

build_contracts:
	docker run --rm -it -v $(shell pwd):/workdir ${PROJECT_SERUM_IMAGE} /bin/bash ./scripts/anchor-build.sh

build: build_js build_contracts

test_relay_unit:
	go build -v ./pkg/...
	go test -v ./pkg/...

test_smoke:
	SELECTED_NETWORKS=solana NETWORK_SETTINGS=$(shell pwd)/tests/e2e/networks.yaml ginkgo tests/e2e/smoke

test_ocr_soak:
	SELECTED_NETWORKS=solana NETWORK_SETTINGS=$(shell pwd)/tests/e2e/networks.yaml ginkgo tests/e2e/soak

test_chaos:
	SELECTED_NETWORKS=solana NETWORK_SETTINGS=$(shell pwd)/tests/e2e/networks.yaml ginkgo tests/e2e/chaos
