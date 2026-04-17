#!/usr/bin/env bash
# Build script for the cat server.
# Usage: ./scripts/build.sh [--test] [--race] [--integration]
#   --test          Run unit tests after build
#   --race          Enable race detector (build & test)
#   --integration   Run integration tests (requires build first)
#
# Exit code 0 = success, non-zero = failure.
# All output (stdout + stderr) is merged for easy capture.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$REPO_ROOT/server"
OUTPUT_DIR="$REPO_ROOT/build"
BINARY_NAME="catserver"

RUN_TESTS=false
RUN_INTEGRATION=false
RACE_FLAG=""

for arg in "$@"; do
  case "$arg" in
    --test) RUN_TESTS=true ;;
    --race) RACE_FLAG="-race" ;;
    --integration) RUN_INTEGRATION=true ;;
    *) echo "Unknown flag: $arg"; exit 1 ;;
  esac
done

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

echo "=== go vet ==="
cd "$SERVER_DIR"
if ! go vet ./... 2>&1; then
  echo "FAIL: go vet found issues"
  exit 1
fi

echo ""
echo "=== check time.Now() usage (M9) ==="
if ! bash "$REPO_ROOT/scripts/check_time_now.sh" 2>&1; then
  exit 1
fi

BUILD_VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS="-X main.buildVersion=${BUILD_VERSION}"

echo ""
echo "=== go build ==="
if ! go build $RACE_FLAG -ldflags "$LDFLAGS" -o "$OUTPUT_DIR/$BINARY_NAME" ./cmd/cat/ 2>&1; then
  echo "FAIL: build failed"
  exit 1
fi
echo "OK: binary at build/$BINARY_NAME"

if [ "$RUN_TESTS" = true ]; then
  echo ""
  echo "=== go test ==="
  if ! go test $RACE_FLAG -count=1 ./... 2>&1; then
    echo "FAIL: tests failed"
    exit 1
  fi
  echo "OK: all tests passed"
fi

if [ "$RUN_INTEGRATION" = true ]; then
  echo ""
  echo "=== integration tests ==="
  if ! go test $RACE_FLAG -tags=integration -count=1 -timeout=120s ./... 2>&1; then
    echo "FAIL: integration tests failed"
    exit 1
  fi
  echo "OK: integration tests passed"
fi

echo ""
echo "BUILD SUCCESS"
