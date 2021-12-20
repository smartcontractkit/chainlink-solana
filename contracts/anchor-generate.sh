#!/usr/bin/env bash

for idl_path_str in "target/idl"/*
do
  IFS='/' read -r -a idl_path <<< "${idl_path_str}"
  IFS='.' read -r -a idl_name <<< "${idl_path[2]}"
  anchor-go -src "${idl_path_str}" -dst generated/"${idl_name[0]}" -codec borsh
done
