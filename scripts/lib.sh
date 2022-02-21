#!/usr/bin/env bash

function modify_program {
  ac=$1
  ocr2=$2
  store=$3

  # Replace existing declare_id!()
  sed -i "s/DzzjdPWNfwHZmzPVxnmqkkMJraYQQRCpgFZajqkqmU6G/$ac/" "${BASH_SOURCE%/*}/../contracts/programs/access-controller/src/lib.rs"
  sed -i "s/HW3ipKzeeduJq6f1NqRCw4doknMeWkfrM4WxobtG3o5v/$ocr2/" "${BASH_SOURCE%/*}/../contracts/programs/ocr2/src/lib.rs"
  sed -i "s/CaH12fwNTKJAG8PxEvo9R96Zc2j8qNHZaFj8ZW49yZNT/$store/" "${BASH_SOURCE%/*}/../contracts/programs/store/src/lib.rs"
}

function build {
  cd "${BASH_SOURCE%/*}/../contracts"
  anchor build
  cd $1
}
