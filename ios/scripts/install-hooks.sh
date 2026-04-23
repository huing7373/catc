#!/usr/bin/env bash
# Install local git hooks for the iOS dev loop.
# Usage: bash ios/scripts/install-hooks.sh [--force]
#   --force   Overwrite existing pre-push hook without prompting.
#
# Installs ios/scripts/git-hooks/pre-push into the repo's git hooks dir.
# Worktree-safe via `git rev-parse --git-path hooks`.
# Story 1A.1 · Source: ios/CatPhone/_bmad-output/implementation-artifacts/1a-1-build-sh-install-hooks-swift-format.md

set -euo pipefail

FORCE=false
for arg in "$@"; do
  case "$arg" in
    --force) FORCE=true ;;
    *) echo >&2 "ERROR: Unknown flag: $arg"; echo >&2 "Usage: bash ios/scripts/install-hooks.sh [--force]"; exit 1 ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HOOK_SOURCE="$REPO_ROOT/ios/scripts/git-hooks/pre-push"

if [ ! -f "$HOOK_SOURCE" ]; then
  echo >&2 "ERROR: hook 模板缺失：$HOOK_SOURCE"
  exit 1
fi

HOOK_DIR_REL="$(git -C "$REPO_ROOT" rev-parse --git-path hooks 2>/dev/null || true)"
if [ -z "$HOOK_DIR_REL" ]; then
  echo >&2 "ERROR: 非 git 仓库（git rev-parse --git-path hooks 失败）"
  exit 1
fi

case "$HOOK_DIR_REL" in
  /*) HOOK_DIR="$HOOK_DIR_REL" ;;
  *)  HOOK_DIR="$REPO_ROOT/$HOOK_DIR_REL" ;;
esac

if [ ! -d "$HOOK_DIR" ]; then
  echo >&2 "ERROR: 非 git 仓库或 hooks 路径不存在 ($HOOK_DIR)"
  exit 1
fi

TARGET="$HOOK_DIR/pre-push"

if [ -e "$TARGET" ] && [ "$FORCE" != true ]; then
  printf "已存在 %s，是否覆盖？(y/N) " "$TARGET"
  read -r reply </dev/tty || reply=""
  case "$reply" in
    [yY]|[yY][eE][sS]) ;;
    *) echo "已取消，未修改 $TARGET"; exit 0 ;;
  esac
fi

cp "$HOOK_SOURCE" "$TARGET"
chmod +x "$TARGET"

echo "hook 已安装；下次 git push 会自动触发 build.sh（不带 --test，跑 lint + build 不跑 xcodebuild test）"
