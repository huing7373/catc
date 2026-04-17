#!/usr/bin/env bash
# CI check: business code must not call time.Now() directly (M9 Clock convention).
# Scanned dirs: server/internal/{service,cron,push,ws,handler}/
# Excluded: _test.go files, middleware/ (legitimate usage for request timing).
#
# Exit code 0 = clean, 1 = violations found.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

DIRS=(
  "$REPO_ROOT/server/internal/service"
  "$REPO_ROOT/server/internal/cron"
  "$REPO_ROOT/server/internal/push"
  "$REPO_ROOT/server/internal/ws"
  "$REPO_ROOT/server/internal/handler"
)

found=0
for dir in "${DIRS[@]}"; do
  if [ ! -d "$dir" ]; then
    continue
  fi
  # grep non-test .go files for time.Now()
  if grep -rn --include='*.go' --exclude='*_test.go' 'time\.Now()' "$dir"; then
    found=1
  fi
done

if [ "$found" -eq 1 ]; then
  echo ""
  echo "FAIL: business code must use clockx.Clock instead of time.Now() (M9)"
  exit 1
fi

echo "OK: no direct time.Now() calls in business code"
