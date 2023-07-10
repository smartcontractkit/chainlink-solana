#!/usr/bin/env bash

function modify_program {
  ac=$1
  ocr2=$2
  store=$3

old_ac=`cat "${BASH_SOURCE%/*}/../contracts/programs/access-controller/src/lib.rs" | grep -E -e 'declare_id\!\(\"[a-zA-Z0-9]+\"\)' | grep -o '".*"' | tr -d '"' | head -1`
old_store=`cat "${BASH_SOURCE%/*}/../contracts/programs/store/src/lib.rs" | grep -E -e 'declare_id\!\(\"[a-zA-Z0-9]+\"\)' | grep -o '".*"' | tr -d '"' | head -1`
old_ocr2=`cat "${BASH_SOURCE%/*}/../contracts/programs/ocr2/src/lib.rs" | grep -E -e 'declare_id\!\(\"[a-zA-Z0-9]+\"\)' | grep -o '".*"' | tr -d '"' | head -1`

  # Replace existing declare_id!()
  sed -i "s/$old_ac/$ac/" "${BASH_SOURCE%/*}/../contracts/programs/access-controller/src/lib.rs"
  sed -i "s/$old_store/$store/" "${BASH_SOURCE%/*}/../contracts/programs/store/src/lib.rs"
  sed -i "s/$old_ocr2/$ocr2/" "${BASH_SOURCE%/*}/../contracts/programs/ocr2/src/lib.rs"
}

function build {
  cd "${BASH_SOURCE%/*}/../contracts"
  anchor build
  cd $1
}
