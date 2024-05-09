#!/usr/bin/env bash
set -eoux pipefail

export RUSTUP_HOME="/root/.rustup"
export FORCE_COLOR=1

WORKDIR=$(pwd)
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
REPO=${SCRIPT_DIR}/../
CONTRACTS=${REPO}/contracts

yarndir=$(which yarn)

function printthepaths () {
    cd "${REPO}"
    echo "yarn: "
    which yarn || true
    echo "go: "
    which go || true
    echo "anchor: "
    which anchor || true
    echo "anchor-go: "
    which anchor-go || true
    cd "${WORKDIR}"
}

# install go
apt-get update
apt-get install -y wget
wget https://golang.org/dl/go1.21.7.linux-amd64.tar.gz
tar -xvf go1.21.7.linux-amd64.tar.gz
mv go /usr/local
export PATH=/usr/local/go/bin:$PATH
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
go version

# install git
apt-get install software-properties-common -y
add-apt-repository ppa:git-core/ppa
apt update
apt install git -y

# install achor-go
go install github.com/gagliardetto/anchor-go@v0.2.3

# initial build
cd "${CONTRACTS}"
yarn install --frozen-lockfile
anchor build

# generate contract artifacts
cd "${REPO}"
./scripts/anchor-go-gen.sh

# check if the go interfaces have changed
git diff --stat --exit-code

# generate program keys
./scripts/programs-keys-gen.sh

# build the contracts with updated keys
cd "${CONTRACTS}"
anchor build
