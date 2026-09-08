#!/bin/sh

set -e

# Change to repo root
cd "$(dirname "$0")/../"

OS=$(uname -s)

# Determine output binary name
case "$OS" in
  *CYGWIN*|*MINGW*|*MSYS*)
    OUTPUT="./build/dance-elite.exe"
    ;;
  *)
    OUTPUT="./build/dance-elite"
    ;;
esac

echo "Building for $OS..."

# Ensure build directories exist and configure a writable Go build cache
mkdir -p ./build/.gocache ./build/.gomodcache
export GOCACHE="$(pwd)/build/.gocache"
export GOMODCACHE="$(pwd)/build/.gomodcache"

# Build the executable
go build -o "$OUTPUT" ./cmd/dance-elite

echo "Build successful!"
echo "To run the application, execute the following command:"
echo "$OUTPUT --port 8090 --dsn /opt/dance-elite/"
