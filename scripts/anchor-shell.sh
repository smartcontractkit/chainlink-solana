#!/usr/bin/env bash

# left this script here for now so people's workflows to use the shell won't change
# just use the makefile now so the actual command is only in one place
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
cd "${SCRIPT_DIR}"/../ && make anchor_shell
