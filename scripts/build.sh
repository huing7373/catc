#!/usr/bin/env bash
# Build script for the cat server (server/ Go module).
#
# Usage: bash scripts/build.sh [--test] [--race] [--coverage] [--integration] [--devtools]
#   --test         Run unit tests after build (`go test -count=1 ./...`)
#   --race         Enable race detector on build + test (requires cgo)
#   --coverage     Emit -cover -coverprofile=build/coverage.out (requires --test)
#   --integration  Run integration tests (-tags=integration, 120s timeout)
#   --devtools     Add -tags devtools to build + test; output binary as build/catserver-dev
#
# Exit code 0 = success, non-zero = failure.
# All output (stdout + stderr) is merged for easy log capture.
#
# Notes:
#   - Entry point is ./cmd/server/ (module path github.com/huing/cat/server).
#   - ldflags inject buildinfo.Commit / buildinfo.BuiltAt for the /version endpoint
#     (see server/internal/buildinfo/buildinfo.go + Story 1.4 AC3).
#   - $(go env GOEXE) expands to ".exe" on Windows, empty on Linux/macOS, so the
#     same script produces catserver on Linux and catserver.exe on Windows.
#   - --devtools and --integration are mutually exclusive (single -tags slot).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$REPO_ROOT/server"
OUTPUT_DIR="$REPO_ROOT/build"
BINARY_NAME="catserver"

RUN_TESTS=false
RUN_COVERAGE=false
RUN_INTEGRATION=false
RUN_DEVTOOLS=false
RACE_FLAG=""

usage() {
  cat <<'EOF'
Usage: bash scripts/build.sh [flags]
  --test         Run unit tests after build
  --race         Enable race detector on build + test
  --coverage     Emit coverage profile (requires --test)
  --integration  Run integration tests (-tags=integration)
  --devtools     Add -tags devtools + build/catserver-dev output
EOF
}

for arg in "$@"; do
  case "$arg" in
    --test)        RUN_TESTS=true ;;
    --race)        RACE_FLAG="-race" ;;
    --coverage)    RUN_COVERAGE=true ;;
    --integration) RUN_INTEGRATION=true ;;
    --devtools)    RUN_DEVTOOLS=true ;;
    *)
      echo "Unknown flag: $arg"
      usage
      exit 1
      ;;
  esac
done

# Mutual exclusivity / prerequisite checks (AC5 / AC6 / AC8)
if [ "$RUN_COVERAGE" = true ] && [ "$RUN_TESTS" = false ]; then
  echo "ERR: --coverage requires --test"
  exit 1
fi
if [ "$RUN_DEVTOOLS" = true ] && [ "$RUN_INTEGRATION" = true ]; then
  echo "ERR: --devtools and --integration are mutually exclusive"
  exit 1
fi

BUILD_TAGS=""
BINARY_SUFFIX=""
if [ "$RUN_DEVTOOLS" = true ]; then
  BUILD_TAGS="-tags devtools"
  BINARY_SUFFIX="-dev"
fi

mkdir -p "$OUTPUT_DIR"

cd "$SERVER_DIR"

echo "=== go vet ==="
# shellcheck disable=SC2086  # intentional word-splitting on BUILD_TAGS
if ! go vet $BUILD_TAGS ./... 2>&1; then
  echo "FAIL: go vet"
  exit 1
fi

COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X 'github.com/huing/cat/server/internal/buildinfo.Commit=${COMMIT}' -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=${BUILT_AT}'"

BINARY_PATH="$OUTPUT_DIR/${BINARY_NAME}${BINARY_SUFFIX}$(go env GOEXE)"

echo ""
echo "=== go build (commit=${COMMIT}, builtAt=${BUILT_AT}) ==="
# shellcheck disable=SC2086  # intentional word-splitting on RACE_FLAG / BUILD_TAGS
if ! go build $RACE_FLAG $BUILD_TAGS -ldflags "$LDFLAGS" -o "$BINARY_PATH" ./cmd/server/ 2>&1; then
  echo "FAIL: go build"
  exit 1
fi
echo "OK: binary at build/${BINARY_NAME}${BINARY_SUFFIX}$(go env GOEXE)"

if [ "$RUN_TESTS" = true ]; then
  COVERAGE_FLAG=""
  if [ "$RUN_COVERAGE" = true ]; then
    COVERAGE_FLAG="-cover -coverprofile=$OUTPUT_DIR/coverage.out"
  fi
  echo ""
  echo "=== go test ==="
  # shellcheck disable=SC2086
  if ! go test -count=1 $RACE_FLAG $BUILD_TAGS $COVERAGE_FLAG ./... 2>&1; then
    echo "FAIL: tests"
    exit 1
  fi
  echo "OK: all tests passed"
  if [ "$RUN_COVERAGE" = true ]; then
    echo "OK: coverage profile at build/coverage.out"
  fi
fi

if [ "$RUN_INTEGRATION" = true ]; then
  echo ""
  echo "=== integration tests ==="
  # shellcheck disable=SC2086
  if ! go test -tags=integration $RACE_FLAG -count=1 -timeout=120s ./... 2>&1; then
    echo "FAIL: integration tests"
    exit 1
  fi
  echo "OK: integration tests passed"
fi

echo ""
echo "BUILD SUCCESS"
