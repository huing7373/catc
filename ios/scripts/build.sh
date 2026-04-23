#!/usr/bin/env bash
# Build script for the iOS app (CatPhone / CatWatch / CatShared / CatCore).
# Usage: bash ios/scripts/build.sh [--test]
#   --test    Run xcodebuild test (iOS Simulator) after build.
#
# Exit code 0 = success, non-zero = failure.
# All failure paths fail-closed: stderr human-readable error + non-zero exit.
# Story 1A.1 · Source: ios/CatPhone/_bmad-output/implementation-artifacts/1a-1-build-sh-install-hooks-swift-format.md

set -euo pipefail

RUN_TESTS=false
for arg in "$@"; do
  case "$arg" in
    --test) RUN_TESTS=true ;;
    *) echo >&2 "ERROR: Unknown flag: $arg"; echo >&2 "Usage: bash ios/scripts/build.sh [--test]"; exit 1 ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IOS_DIR="$REPO_ROOT/ios"
PROJECT_PATH="$IOS_DIR/Cat.xcodeproj"
SCHEME="CatPhone"
STRICT_SWIFT_FLAGS="OTHER_SWIFT_FLAGS=-warnings-as-errors"

require_tool() {
  local tool="$1"
  local install_hint="$2"
  if ! command -v "$tool" >/dev/null 2>&1; then
    {
      echo "ERROR: $tool 未安装。请先："
      echo "  $install_hint"
      echo "然后重试 bash ios/scripts/build.sh"
    } >&2
    exit 1
  fi
}

require_tool xcodegen "brew install xcodegen swift-format"
require_tool swift-format "brew install xcodegen swift-format"
require_tool xcodebuild "xcode-select --install  # 或从 App Store 安装 Xcode"

echo "=== xcodegen generate ==="
if ! (cd "$IOS_DIR" && xcodegen generate); then
  echo >&2 "FAIL: xcodegen generate failed (check ios/project.yml)"
  exit 1
fi

echo ""
echo "=== xcodebuild build ($SCHEME / Debug) ==="
if ! xcodebuild build \
    -project "$PROJECT_PATH" \
    -scheme "$SCHEME" \
    -configuration Debug \
    "$STRICT_SWIFT_FLAGS"; then
  echo >&2 "FAIL: xcodebuild build failed"
  exit 1
fi

echo ""
echo "=== swift-format lint --strict ==="
if ! swift-format lint --strict --recursive "$IOS_DIR/"; then
  echo >&2 "FAIL: swift-format lint reported violations"
  exit 1
fi

if [ "$RUN_TESTS" = true ]; then
  # Resolve test destination:
  #   1. IOS_TEST_DESTINATION env var (explicit override · CI/canonical use)
  #   2. iPhone 15 (PRD AC2 literal) when its simulator runtime is installed
  #   3. First available iPhone simulator on this machine (graceful dev-loop fallback)
  #   4. Fail-fast with install hint if no iPhone simulator is found at all
  if [ -n "${IOS_TEST_DESTINATION:-}" ]; then
    DESTINATION="$IOS_TEST_DESTINATION"
  elif xcrun simctl list devices available 2>/dev/null | grep -qE '^[[:space:]]+iPhone 15 \('; then
    DESTINATION="platform=iOS Simulator,name=iPhone 15"
  else
    AUTO_NAME=$(xcrun simctl list devices available 2>/dev/null \
      | awk '/^[[:space:]]+iPhone[[:space:]]/ { sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+\(.*$/, ""); print; exit }')
    if [ -z "$AUTO_NAME" ]; then
      {
        echo "ERROR: 未找到任何可用 iPhone 模拟器 runtime。请二选一："
        echo "  · Xcode → Settings → Components → 安装一个 iOS Simulator runtime"
        echo "  · export IOS_TEST_DESTINATION='platform=iOS Simulator,name=...'  # 指定自定义 destination"
      } >&2
      exit 1
    fi
    DESTINATION="platform=iOS Simulator,name=$AUTO_NAME"
    echo ">>> 未找到 iPhone 15 模拟器；自动选用 \"$AUTO_NAME\"（如需指定其它，export IOS_TEST_DESTINATION='platform=iOS Simulator,name=...'）"
  fi

  echo ""
  echo "=== xcodebuild test (destination: $DESTINATION) ==="
  if ! xcodebuild test \
      -project "$PROJECT_PATH" \
      -scheme "$SCHEME" \
      -destination "$DESTINATION" \
      "$STRICT_SWIFT_FLAGS"; then
    echo >&2 "FAIL: xcodebuild test failed (override destination via IOS_TEST_DESTINATION env var)"
    exit 1
  fi
fi

echo ""
echo "BUILD SUCCESS"
