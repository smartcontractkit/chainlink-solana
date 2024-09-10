#!/usr/bin/env bash

set -e

ACCESS_CONTROLLER_PROGRAM_ID=$1
STORE_PROGRAM_ID=$2
OCR2_PROGRAM_ID=$3

echo "Current directory: $(pwd)"
echo "Contents of ./contracts directory:"
ls ./contracts

WORKSPACE=$(mktemp -d)
echo "Temporary workspace: $WORKSPACE"

cp -r ./contracts $WORKSPACE/

cd $WORKSPACE/contracts

echo "Workspace contents:"
ls $WORKSPACE/contracts

# TODO: add linux support
sed -i '' "s|declare_id!(\"[^\"]*\");|declare_id!(\"$ACCESS_CONTROLLER_PROGRAM_ID\");|" "$WORKSPACE/contracts/programs/access-controller/src/lib.rs"
sed -i '' "s|declare_id!(\"[^\"]*\");|declare_id!(\"$STORE_PROGRAM_ID\");|" "$WORKSPACE/contracts/programs/store/src/lib.rs"
sed -i '' "s|declare_id!(\"[^\"]*\");|declare_id!(\"$OCR2_PROGRAM_ID\");|" "$WORKSPACE/contracts/programs/ocr_2/src/lib.rs"

docker run --rm -it -v $(pwd):/workdir backpackapp/build:v0.29.0 /bin/bash -c "anchor build"

echo "Build complete. Artifacts are located in $WORKSPACE/contracts/target/deploy"
ls $WORKSPACE/contracts/target/deploy

mkdir -p $WORKSPACE/artifacts/
cp -r $WORKSPACE/contracts/target/deploy/*.so $WORKSPACE/artifacts/
cp -r $WORKSPACE/contracts/target/idl/*.json $WORKSPACE/artifacts/

echo "Artifacts and metadata copied to $WORKSPACE/artifacts/"
