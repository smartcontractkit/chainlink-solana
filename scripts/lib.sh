#!/usr/bin/env bash

function modify_program {
  ac=$1
  ocr2=$2
  store=$3

  # Replace existing declare_id!()
  sed -i "s/9xi644bRR8birboDGdTiwBq3C7VEeR7VuamRYYXCubUW/$ac/" "${BASH_SOURCE%/*}/../contracts/programs/access-controller/src/lib.rs"
  sed -i "s/HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny/$store/" "${BASH_SOURCE%/*}/../contracts/programs/store/src/lib.rs"
  sed -i "s/cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ/$ocr2/" "${BASH_SOURCE%/*}/../contracts/programs/ocr2/src/lib.rs"
}

function build {
  cd "${BASH_SOURCE%/*}/../contracts"
  anchor build
  cd $1
}
