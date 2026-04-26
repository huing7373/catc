#!/usr/bin/env bash
# iphone/scripts/build.sh
# Story 2.7 · ADR-0002 §3.4 落地：iPhone App 构建 / 测试 wrapper。
#
# Usage: bash iphone/scripts/build.sh [--test] [--uitest] [--clean] [--coverage-export]
#   --test              加跑单元测试（PetAppTests scheme，xcodebuild test）
#   --uitest            加跑 UI 测试（PetAppUITests，xcodebuild test -only-testing）
#   --clean             加跑 xcodebuild clean + 删 iphone/build/DerivedData
#   --coverage-export   跑完测试后调 xcrun xccov 导出 coverage 到 iphone/build/coverage.json
#                       （要求 --test 或 --uitest 之一）
#
# Exit code 0 = success, non-zero = failure.
# 全部 stdout + stderr merge 便于 log 捕获。
#
# Notes:
#   - 入口工程：iphone/PetApp.xcodeproj（由 xcodegen 从 iphone/project.yml 生成）
#   - 默认 scheme：PetApp（含 PetApp / PetAppTests / PetAppUITests 三个 target）
#   - artifacts 路径：iphone/build/{test-results.xcresult, DerivedData/, coverage.json}
#     —— 与 server 端 build/ 严格隔离（ADR-0002 §3.4 已知坑第 4 条）
#   - destination 三段 fallback：iPhone 17,OS=latest → OS=latest → xcrun simctl 第一个可用
#     （ADR-0002 §3.4 已知坑第 2 条：Xcode 16 / 26 默认机型不一致）
#   - --uitest 与 --test 不互斥：可同时跑（XCUITest scheme 与 unit test scheme 独立 invocation）

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IPHONE_DIR="$REPO_ROOT/iphone"
PROJECT_PATH="$IPHONE_DIR/PetApp.xcodeproj"
SCHEME="PetApp"
OUTPUT_DIR="$IPHONE_DIR/build"
DERIVED_DATA="$OUTPUT_DIR/DerivedData"
TEST_RESULTS="$OUTPUT_DIR/test-results.xcresult"
COVERAGE_JSON="$OUTPUT_DIR/coverage.json"

RUN_TESTS=false
RUN_UITESTS=false
RUN_CLEAN=false
EXPORT_COVERAGE=false

usage() {
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
}

for arg in "$@"; do
  case "$arg" in
    --test)             RUN_TESTS=true ;;
    --uitest)           RUN_UITESTS=true ;;
    --clean)            RUN_CLEAN=true ;;
    --coverage-export)  EXPORT_COVERAGE=true ;;
    -h|--help)          usage; exit 0 ;;
    *)
      echo >&2 "ERROR: 未知参数：$arg"
      usage
      exit 1
      ;;
  esac
done

# 前置校验：--coverage-export 要求 --test 或 --uitest
# coverage 源选择规则（lesson 2026-04-26-build-script-flag-matrix.md）：
#   --test 单独           → 从 unit bundle 读
#   --uitest 单独         → 从 ui bundle 读
#   --test + --uitest     → 从 unit bundle 读（unit 覆盖 production code 是主路径）
#   两者都没 + 仅 export  → preflight 拒
if [ "$EXPORT_COVERAGE" = true ] && [ "$RUN_TESTS" = false ] && [ "$RUN_UITESTS" = false ]; then
  echo >&2 "ERROR: --coverage-export 要求 --test 或 --uitest"
  exit 1
fi

# require_tool helper（参考 server/scripts/build.sh 风格 + iphone/scripts/install-hooks.sh）
require_tool() {
  local tool="$1"
  local install_hint="$2"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo >&2 "ERROR: $tool 未安装"
    echo >&2 "  安装：$install_hint"
    exit 1
  fi
}

require_tool xcodegen   "brew install xcodegen"
require_tool xcodebuild "（macOS Xcode 自带；运行 xcode-select --install）"
require_tool xcrun      "（macOS Xcode 自带）"

mkdir -p "$OUTPUT_DIR"
mkdir -p "$DERIVED_DATA"

cd "$IPHONE_DIR"

# === xcodegen ===（每次跑都 regen，与 Story 2.4 / 2.5 既有惯例一致）
echo "=== xcodegen generate ==="
if ! xcodegen generate 2>&1; then
  echo "FAIL: xcodegen generate"
  exit 1
fi
echo "OK: PetApp.xcodeproj generated"

# === clean（可选）===
if [ "$RUN_CLEAN" = true ]; then
  echo ""
  echo "=== xcodebuild clean ==="
  xcodebuild clean -project "$PROJECT_PATH" -scheme "$SCHEME" 2>&1 || true
  rm -rf "$DERIVED_DATA"
  rm -rf "$TEST_RESULTS"
  rm -rf "${TEST_RESULTS%.xcresult}-ui.xcresult"
  echo "OK: clean done"
fi

# === destination 三段 fallback 解析（ADR-0002 §3.4 已知坑第 2 条）===
DESTINATION_PRIMARY="platform=iOS Simulator,name=iPhone 17,OS=latest"
DESTINATION_SECONDARY="platform=iOS Simulator,OS=latest"
RESOLVED_DESTINATION=""

# fallback 链：尝试用 xcodebuild -showdestinations 判断 primary 是否可解析
# （比真跑 build 失败再 fallback 快）
#
# 关键：`xcodebuild -showdestinations` 输出有两段——
#   "Available destinations for the \"<scheme>\" scheme:"
#   "Ineligible destinations for the \"<scheme>\" scheme:"
# Ineligible 段列出运行时缺失 / 不兼容的 destination；如果 grep 整段输出，
# iPhone 17 哪怕只在 Ineligible 段出现脚本也会选它，后续 build 必定失败。
# → 用 awk 范围选择只看 Available 段
# （lesson 2026-04-26-xcodebuild-showdestinations-section-aware.md）
SHOWDEST_OUTPUT="$(xcodebuild -project "$PROJECT_PATH" -scheme "$SCHEME" -showdestinations 2>/dev/null || true)"
AVAILABLE_DESTINATIONS="$(echo "$SHOWDEST_OUTPUT" | awk '/Available destinations/{flag=1; next} /Ineligible destinations/{flag=0} flag')"

if echo "$AVAILABLE_DESTINATIONS" | grep -q "iPhone 17"; then
  RESOLVED_DESTINATION="$DESTINATION_PRIMARY"
elif echo "$AVAILABLE_DESTINATIONS" | grep -q "iOS Simulator"; then
  RESOLVED_DESTINATION="$DESTINATION_SECONDARY"
else
  # 第三段 fallback：xcrun simctl 取第一个可用 simulator UUID
  FALLBACK_UUID="$(xcrun simctl list devices iOS available 2>/dev/null | grep -Eo '\([0-9A-F-]{36}\)' | head -1 | tr -d '()')"
  if [ -z "$FALLBACK_UUID" ]; then
    echo "FAIL: 无法解析任何 iOS Simulator destination；请检查 Xcode 安装与 iOS Simulator runtime"
    exit 1
  fi
  RESOLVED_DESTINATION="platform=iOS Simulator,id=$FALLBACK_UUID"
fi

echo ""
echo "=== resolved destination: $RESOLVED_DESTINATION ==="

# === xcodebuild build ===（默认行为：vet 等价物 + build）
echo ""
echo "=== xcodebuild build ==="
if ! xcodebuild build \
    -project "$PROJECT_PATH" \
    -scheme "$SCHEME" \
    -destination "$RESOLVED_DESTINATION" \
    -derivedDataPath "$DERIVED_DATA" \
    2>&1; then
  echo "FAIL: xcodebuild build"
  exit 1
fi
echo "OK: build succeeded"

# === xcodebuild test（unit）===
if [ "$RUN_TESTS" = true ]; then
  echo ""
  echo "=== xcodebuild test (unit, scheme=$SCHEME) ==="
  # 删除既有 .xcresult，避免 xcodebuild 拒写
  rm -rf "$TEST_RESULTS" 2>/dev/null || true
  if ! xcodebuild test \
      -project "$PROJECT_PATH" \
      -scheme "$SCHEME" \
      -destination "$RESOLVED_DESTINATION" \
      -resultBundlePath "$TEST_RESULTS" \
      -derivedDataPath "$DERIVED_DATA" \
      -enableCodeCoverage YES \
      -only-testing:PetAppTests \
      2>&1; then
    echo "FAIL: unit tests"
    exit 1
  fi
  echo "OK: unit tests passed"
fi

# === xcodebuild test (UI) ===
if [ "$RUN_UITESTS" = true ]; then
  echo ""
  echo "=== xcodebuild test (ui, scheme=$SCHEME) ==="
  UI_RESULTS="${TEST_RESULTS%.xcresult}-ui.xcresult"
  rm -rf "$UI_RESULTS" 2>/dev/null || true
  # `-enableCodeCoverage YES` 必须显式开启：xcodebuild 默认 NO，UI bundle 不开就没 coverage 数据
  # → `--uitest --coverage-export` 组合下 xccov 输出 lineCoverage 全 0（lesson 2026-04-26-build-script-flag-matrix.md）
  if ! xcodebuild test \
      -project "$PROJECT_PATH" \
      -scheme "$SCHEME" \
      -destination "$RESOLVED_DESTINATION" \
      -resultBundlePath "$UI_RESULTS" \
      -derivedDataPath "$DERIVED_DATA" \
      -enableCodeCoverage YES \
      -only-testing:PetAppUITests \
      2>&1; then
    echo "FAIL: ui tests"
    exit 1
  fi
  echo "OK: ui tests passed"
fi

# === coverage 导出 ===
if [ "$EXPORT_COVERAGE" = true ]; then
  # 智能选择 coverage 源 bundle：unit 优先（覆盖 production code 是主路径）
  # 仅 --uitest 时退回 UI bundle —— 否则脚本会 cat 一个不存在的 .xcresult
  COVERAGE_SOURCE=""
  if [ "$RUN_TESTS" = true ]; then
    COVERAGE_SOURCE="$TEST_RESULTS"
  elif [ "$RUN_UITESTS" = true ]; then
    COVERAGE_SOURCE="${TEST_RESULTS%.xcresult}-ui.xcresult"
  fi

  echo ""
  echo "=== xcrun xccov view --report --json (source: $COVERAGE_SOURCE) ==="
  if ! xcrun xccov view --report --json "$COVERAGE_SOURCE" > "$COVERAGE_JSON" 2>&1; then
    echo "FAIL: coverage export"
    exit 1
  fi
  echo "OK: coverage at iphone/build/coverage.json"
fi

echo ""
echo "BUILD SUCCESS"
