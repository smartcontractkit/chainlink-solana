#!/usr/bin/env bash

set -e

# Function to print usage
print_usage() {
  echo "Usage: $0 --output-dir <output_directory> <program1_name> <program1_id> [<program2_name> <program2_id> ...]"
  echo "Example: $0 --output-dir ./dist/contracts access-controller ACCE55C0NTR0LLRED000000000000000 store 5T0REP2OGR4M0000000000000000000"
}

# Check if at least three arguments are provided (including --output-dir)
if [ "$#" -lt 3 ]; then
  echo "Error: Insufficient arguments."
  print_usage
  exit 1
fi

# Parse arguments
if [ "$1" == "--output-dir" ]; then
  OUTPUT_DIR="$2"
  shift 2
else
  echo "Error: --output-dir must be specified."
  print_usage
  exit 1
fi

# Check if the number of remaining arguments is even
if [ $((${#} % 2)) -ne 0 ]; then
  echo "Error: Each program must have a name and an ID."
  print_usage
  exit 1
fi

echo "Current directory: $(pwd)"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "Script directory: $SCRIPT_DIR"

# Copy the contracts to the output directory
echo "Copying contracts to output directory: $OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"
cp -R "$SCRIPT_DIR/contracts" "$OUTPUT_DIR"

WORKSPACE="$OUTPUT_DIR/contracts"

# Ensure all files are writable
chmod -R u+rw "$WORKSPACE"

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
    echo "Error: Program file not found for $program_name at $program_path"
    exit 1
  fi
done

# Compile the programs
docker run --rm -v "$WORKSPACE":/workdir backpackapp/build:v0.29.0 /bin/bash -c "anchor build"

echo "Build complete. Copying artifacts to $OUTPUT_DIR"

# Copy artifacts to the specified output directory
# TODO: only copy requested artifacts - but folder name is somehow different...
cp "$WORKSPACE/target/deploy/"*.so "$OUTPUT_DIR/"

echo "Artifacts and program ids copied to $OUTPUT_DIR"

# clean up
rm -rf "$WORKSPACE"
