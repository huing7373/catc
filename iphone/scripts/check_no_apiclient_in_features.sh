#!/usr/bin/env bash
# iphone/scripts/check_no_apiclient_in_features.sh
# Story 37.13 AC5 落地：静态校验 Features/{Home,Room,Wardrobe,Friends,Profile}/Views/ +
# Shared/Modals/ + Core/DesignSystem/ 内 import / 直接 new APIClient / Repository / UseCase
# 类型（除显式 RealViewModel wire 模式）.
#
# 守护 ADR-0010 View ↔ ViewModel 解耦边界：
#   - View 层 + Modal + DesignSystem 不直接调网络 / 持久化层；只持 ViewModel 引用.
#   - RealViewModel 内 wire 时可用（合法）—— 通过文件 path（仅扫 Views/ 子目录）天然排除
#     `*ViewModels/Real*.swift` —— Real / Mock VM 文件不在 Views/ 下.
#
# 已知边界：
#   - 用 grep 文本匹配, 不解析 AST.
#   - 显式列每个类型而非泛 regex 兜底（防误报）.
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

# 违规 pattern（合理写法是 ViewModel 持引用 + View 仅持 ViewModel）.
# 显式列已落地的 Repository / UseCase 类型（防 view body 内出现 helper 名带 "Repository" 后缀的误报）.
VIOLATION_PATTERN='(import APIClient|APIClient\(|APIClientProtocol|HomeRepository\(|AuthRepository\(|RoomRepository\(|WardrobeRepository\(|FriendsRepository\(|ProfileRepository\(|LoadHomeUseCase\(|JoinRoomUseCase\(|GuestLoginUseCase\(|PingUseCase\()'

violations=0

for dir in "${SCAN_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        continue  # DesignSystem 等目录可能本期还未存在
    fi
    while IFS= read -r -d '' file; do
        # grep -nE 输出违规行（含行号）
        matches="$(grep -nE "$VIOLATION_PATTERN" "$file" || true)"
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
    echo "Fix: 把 APIClient / Repository / UseCase 调用从 View / Modal / DesignSystem 移到对应的 ViewModel" >&2
    exit 1
fi

echo "✅ View/Modal/DesignSystem APIClient isolation OK"
