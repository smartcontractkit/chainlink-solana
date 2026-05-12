#!/usr/bin/env bash

set -e

for idl_path_str in "contracts/target/idl"/*
do
  filename=$(basename "$idl_path_str")

  if [[ "$filename" == "dummy_receiver.json" ]]; then
    echo "Skipping $filename"
    continue
  fi
  
  IFS='/' read -r -a idl_path <<< "${idl_path_str}"
  IFS='.' read -r -a idl_name <<< "${idl_path[3]}"
  rm -rf ./contracts/generated/"${idl_name}"
  anchor-go -idl "${idl_path_str}" -output ./contracts/generated/"${idl_name}"  -no-go-mod
done
