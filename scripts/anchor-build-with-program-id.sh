#!/usr/bin/env bash

set -e

ACCESS_CONTROLLER_PROGRAM_ID=$1
STORE_PROGRAM_ID=$2
OCR2_PROGRAM_ID=$3

echo "Current directory: $(pwd)"
echo "Contents of ./contracts directory:"

WORKSPACE=./contracts/
echo "Workspace contents:"
ls $WORKSPACE

replaceDeclaredProgramId() {
  local file="$1"
  local program_id="$2"

  if [ "$(uname -s)" = "Darwin" ]; then
    # macOS (Darwin) version
    sed -i '' "/^declare_id!/s/\"[^\"]*\"/\"$program_id\"/" "$file"
  else
    # Linux and other Unix-like systems
    sed -i "/^declare_id!/s/\"[^\"]*\"/\"$program_id\"/" "$file"
  fi
}

replaceDeclaredProgramId "$WORKSPACE/programs/access-controller/src/lib.rs" "$ACCESS_CONTROLLER_PROGRAM_ID"
replaceDeclaredProgramId "$WORKSPACE/programs/store/src/lib.rs" "$STORE_PROGRAM_ID"
replaceDeclaredProgramId "$WORKSPACE/programs/ocr_2/src/lib.rs" "$OCR2_PROGRAM_ID"

docker run --rm -it -v $WORKSPACE:/workdir backpackapp/build:v0.29.0 /bin/bash -c "anchor build"

echo "Build complete. Artifacts are located in $WORKSPACE/target/deploy"
ls $WORKSPACE/target/deploy

mkdir -p $WORKSPACE/artifacts/
cp -r $WORKSPACE/target/deploy/*.so $WORKSPACE/artifacts/
cp -r $WORKSPACE/target/idl/*.json $WORKSPACE/artifacts/

echo "Artifacts and metadata copied to $WORKSPACE/artifacts/"
