#!/usr/bin/env bash

# note call from repo root
set -e
source ./scripts/lib.sh
source ./gauntlet/packages/gauntlet-solana-contracts/networks/.env.staging

modify_program $PROGRAM_ID_ACCESS_CONTROLLER $PROGRAM_ID_OCR2 $PROGRAM_ID_STORE

# build artifacts
build
