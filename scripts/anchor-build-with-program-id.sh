#!/usr/bin/env bash

set -e

# Function to print usage
print_usage() {
  echo "Usage: $0 <program1_name> <program1_id> [<program2_name> <program2_id> ...]"
  echo "Example: $0 access-controller ACCE55C0NTR0LLRED000000000000000 store 5T0REP2OGR4M0000000000000000000"
}

# Check if at least two arguments are provided
if [ "$#" -lt 2 ]; then
  echo "Error: Insufficient arguments."
  print_usage
  exit 1
fi

# Check if the number of arguments is even
if [ $((${#} % 2)) -ne 0 ]; then
  echo "Error: Each program must have a name and an ID."
  print_usage
  exit 1
fi

echo "Current directory: $(pwd)"

WORKSPACE=./contracts/
echo "Workspace contents:"
ls $WORKSPACE

replaceDeclaredProgramId() {
  local file="$1"
  local program_id="$2"

  if [ "$(uname -s)" = "Darwin" ]; then
    sed -i '' "/^declare_id!/s/\"[^\"]*\"/\"$program_id\"/" "$file"
  else
    sed -i "/^declare_id!/s/\"[^\"]*\"/\"$program_id\"/" "$file"
  fi
}

# Process each program
while [ "$#" -gt 0 ]; do
  program_name="$1"
  program_id="$2"
  shift 2

  program_path="$WORKSPACE/programs/$program_name/src/lib.rs"
  if [ -f "$program_path" ]; then
    echo "Processing $program_name with ID $program_id"
    replaceDeclaredProgramId "$program_path" "$program_id"
  else
    echo "Warning: Program file not found for $program_name"
    exit 1
  fi
done

# Compile the programs
# NOTE:Currently Anchor compiles whole workspace, but in the future we may need to compile only the programs as required
docker run --rm -it -v $WORKSPACE:/workdir backpackapp/build:v0.29.0 /bin/bash -c "anchor build"

echo "Build complete. Artifacts are located in $WORKSPACE/target/deploy"
ls $WORKSPACE/target/deploy

mkdir -p $WORKSPACE/artifacts/
cp -r $WORKSPACE/target/deploy/*.so $WORKSPACE/artifacts/
cp -r $WORKSPACE/target/idl/*.json $WORKSPACE/artifacts/

echo "Artifacts and metadata copied to $WORKSPACE/artifacts/"
