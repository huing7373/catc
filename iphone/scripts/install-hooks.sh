#!/usr/bin/env bash
# iphone/scripts/install-hooks.sh
# Story 2.2 · ADR-0002 §3.3 方案 D 阶段 2：iPhone 端 git hooks 安装入口。
#
# Usage:
#   bash iphone/scripts/install-hooks.sh [--force]
#     --force   覆盖已存在的 .git/hooks/pre-commit，不弹确认。
#
# 行为：
#   1. 验证 swift-format 已安装且版本号 startsWith 602.（ADR-0002 §4 + §6 TODO）
#   2. 把 iphone/scripts/git-hooks/pre-commit 拷贝到 .git/hooks/pre-commit
#   3. chmod +x，让 hook 可执行
#
# 注：本 story 阶段 hook 内容是占位 exit 0；真实 swift-format / lint 调用作为 tech debt
# 在后续 story 视需要扩展（参考本文件头与 git-hooks/pre-commit 的注释）。

set -euo pipefail

FORCE=false
for arg in "$@"; do
  case "$arg" in
    --force) FORCE=true ;;
    -h|--help)
      sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo >&2 "ERROR: 未知参数：$arg"
      echo >&2 "Usage: bash iphone/scripts/install-hooks.sh [--force]"
      exit 1
      ;;
  esac
done

# ----------------------- swift-format 版本验证 -----------------------

require_tool() {
  local tool="$1"
  local install_hint="$2"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo >&2 "ERROR: $tool 未安装"
    echo >&2 "  安装：$install_hint"
    exit 1
  fi
}

require_tool swift-format "brew install swift-format"

# ADR-0002 §4 锁定 swift-format 主版本号 602.x（unversioned brew formula）。
# 当 brew stable 升级到 603.x 时，dev 需评估是否更新本脚本 + ADR §4。
SWIFT_FORMAT_VERSION_RAW="$(swift-format --version 2>&1 | head -1 | tr -d '[:space:]')"
case "$SWIFT_FORMAT_VERSION_RAW" in
  602.*) ;;
  *)
    echo >&2 "ERROR: swift-format 版本不匹配：$SWIFT_FORMAT_VERSION_RAW"
    echo >&2 "  期望：602.* （ADR-0002 §4 锁定）"
    echo >&2 "  操作：brew upgrade swift-format 或评估更新 ADR §4 + 本脚本"
    exit 1
    ;;
esac

# ----------------------- 拷贝 hook 文件 -----------------------

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HOOK_SOURCE="$REPO_ROOT/iphone/scripts/git-hooks/pre-commit"

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
  echo >&2 "ERROR: hooks 路径不存在 ($HOOK_DIR)"
  exit 1
fi

TARGET="$HOOK_DIR/pre-commit"

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

echo "✅ pre-commit hook 已安装到 $TARGET"
echo "   swift-format 版本：$SWIFT_FORMAT_VERSION_RAW"
echo ""
echo "提示（ADR-0002 §3.3 方案 D 阶段 2 过渡期）："
echo "  如此前已 run 旧 ios/scripts/install-hooks.sh，应手工卸载 .git/hooks/pre-commit / pre-push 后再 run 新版。"
