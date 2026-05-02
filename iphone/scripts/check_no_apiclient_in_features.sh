#!/usr/bin/env bash
# iphone/scripts/check_no_apiclient_in_features.sh
# Story 37.13 AC5 落地（r2 加固）：静态校验 Features/{Home,Room,Wardrobe,Friends,Profile}/Views/ +
# Shared/Modals/ + Core/DesignSystem/ 内任何 APIClient / *Repository / *UseCase / Default*
# 类型 token 出现（覆盖 import / 构造调用 / 属性类型 / 参数类型 / protocol 类型注解 / 具体实现引用）.
#
# 守护 ADR-0010 View ↔ ViewModel 解耦边界：
#   - View 层 + Modal + DesignSystem 不直接调网络 / 持久化层；只持 ViewModel 引用.
#   - RealViewModel 内 wire 时可用（合法）—— 通过文件 path（仅扫 Views/ 子目录）天然排除
#     `*ViewModels/Real*.swift` —— Real / Mock VM 文件不在 Views/ 下.
#
# 已知边界：
#   - 用 grep 文本匹配, 不解析 AST.
#   - 启发式跳过纯 comment 行（`//` / `///` 开头）—— forward-reference 注释 / 文档字符串不算违规.
#   - 不跨多行（多行 trailing closure 形参声明可能漏掉，但在 view body 内极罕见）.
#
# r2 加固背景：r1 版本仅匹配 `TypeName(` 构造调用（如 `LoadHomeUseCase(` / `HomeRepository(`），
# 漏掉常见 view-layer regression 形式 ——
#   - `let useCase: LoadHomeUseCaseProtocol`           (property type annotation)
#   - `func wireUp(useCase: SomeRepository)`           (parameter type)
#   - `var fallback: DefaultLoadHomeUseCase`           (concrete impl reference)
#   - `import APIClient`（已覆盖）
# r2 改为 token-level 广泛匹配（任何 `\b<Word>UseCase\b` / `\b<Word>Repository\b` / `\bAPIClient\b`），
# 加 comment-line skip 排除 doc 引用.
#
# Usage: bash iphone/scripts/check_no_apiclient_in_features.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCAN_DIRS=(
    "$REPO_ROOT/iphone/PetApp/Features/Home/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Room/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Wardrobe/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Friends/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Profile/Views"
    "$REPO_ROOT/iphone/PetApp/Shared/Modals"
    "$REPO_ROOT/iphone/PetApp/Core/DesignSystem"
)

# 违规 token pattern（r2 加固）：
#   - `[A-Z][A-Za-z0-9]*UseCase` 后可选 `Protocol` —— 覆盖 LoadHomeUseCase / LoadHomeUseCaseProtocol / DefaultLoadHomeUseCase
#   - `[A-Z][A-Za-z0-9]*Repository` 后可选 `Protocol` —— 覆盖 HomeRepository / HomeRepositoryProtocol / DefaultHomeRepository
#   - `APIClient` 后可选 `Protocol` —— 覆盖 APIClient / APIClientProtocol
# `\b` 词边界防 helper 名内含子串误报（如 `myUseCaseHelper` 不会触发，因为前面无词边界 + 大写起始约束）.
# 命名约定：所有相关类型必为 PascalCase 起始. 触发条件：行不是 `//` / `///` comment 起始.
VIOLATION_PATTERN='\b([A-Z][A-Za-z0-9]*UseCase|[A-Z][A-Za-z0-9]*Repository|APIClient)(Protocol)?\b'

violations=0

for dir in "${SCAN_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        continue  # DesignSystem 等目录可能本期还未存在
    fi
    while IFS= read -r -d '' file; do
        # 用 awk 跳过纯注释行（`//` / `///` 起始，允许前导空白），
        # 然后 grep -nE 输出违规行（带原始行号）.
        matches="$(awk '!/^[[:space:]]*\/\//{print NR":"$0}' "$file" \
            | grep -E "$VIOLATION_PATTERN" || true)"
        if [ -n "$matches" ]; then
            while IFS= read -r match; do
                [ -z "$match" ] && continue
                echo "VIOLATION: $file: $match" >&2
                violations=$((violations + 1))
            done <<< "$matches"
        fi
    done < <(find "$dir" -name "*.swift" -type f -print0)
done

if [ "$violations" -gt 0 ]; then
    echo "" >&2
    echo "❌ Total violations: $violations" >&2
    echo "Fix: 把 APIClient / Repository / UseCase 调用 / 类型引用从 View / Modal / DesignSystem 移到对应的 ViewModel" >&2
    echo "Hint: r2 加固后覆盖 property type / parameter type / protocol type / concrete impl 等所有 token 形式." >&2
    exit 1
fi

echo "✅ View/Modal/DesignSystem APIClient isolation OK"
