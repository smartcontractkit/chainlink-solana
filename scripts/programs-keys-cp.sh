#!/usr/bin/env bash

set -e

network=${1:-localnet}

cp contracts/artifacts/$network/*.json contracts/target/deploy
