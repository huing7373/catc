#!/usr/bin/env bash
# iphone/scripts/check_a11y_coverage.sh
# Story 37.13 AC4 落地：静态校验 Features/ + Shared/Modals/ 内 SwiftUI View body 含
# 交互元素（Button / Toggle / TextField / NavigationLink / Picker / Slider /
# TextEditor）但未挂 accessibilityIdentifier(...) 注解的违规点。
#
# 设计原则（合规兜底，非完整 lint）：
#   - 走 grep 文本匹配, 不解析 AST（避免引入 swift-syntax 依赖）.
#   - 已知漏报：①嵌套 ViewBuilder 内的 Button 漏挂时若整层挂 a11y 会 false negative；
#     ②.contextMenu / .swipeActions 等次级菜单内的 Button 不算交互（确实用户主动入口少，不强求）.
#   - 已知误报：①@ViewBuilder helper 函数返回 `some View` 内的 Button 若 a11y 在
#     caller 挂则本脚本报为漏挂.
#
# 这两类误报漏报的边界：脚本仅作为 "新增 view 时漏挂 a11y 第一道防线"，
# 真正的覆盖完整性仍依赖 AC2 落地的 47 处常量替换（dev 必须先做 AC2 再跑本脚本，
# 跑通后说明本期 baseline OK；后续 story 加新 view 时跑本脚本看是否冒新违规）.
#
# Usage: bash iphone/scripts/check_a11y_coverage.sh
# Exit code 0 = no violations, non-zero = violations listed on stderr.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCAN_DIRS=(
    "$REPO_ROOT/iphone/PetApp/App"
    "$REPO_ROOT/iphone/PetApp/Features"
    "$REPO_ROOT/iphone/PetApp/Shared/Modals"
)

# 交互元素 grep pattern（含常见的, 漏报场景在 header doc 已声明）.
# 关键：用 ([^A-Za-z_]|^) 前置（非标识符字符 / 行首）避免匹配
# `actionButton(` / `tabButton(` 等本身已在 helper body 内挂 .accessibilityIdentifier
# 的 helper callsite false positive.
#
# 但是！对于 helper body 内 *不* 挂 .accessibilityIdentifier 的 wrapper（caller 必须挂），
# 必须把 helper 名加进 pattern，否则 caller 漏挂时 CI 静默放行。当前已知此类 wrapper：
#   - PrimaryButton（HomeView / JoinRoomModal / RoomScaffoldView / WardrobeScaffoldView /
#     FriendsScaffoldView 大量使用，body 内只是 SwiftUI Button 包一层样式，a11y 由 caller 挂）
#   - headerIconButton（ProfileScaffoldView 私有 helper，body 内不挂 a11y → caller 必须挂）
# 后续如再加同模式 wrapper, 同步把 wrapper 名追加到下面 pattern.
INTERACTIVE_PATTERN='(^|[^A-Za-z_])(Button[[:space:]]*\(|Button[[:space:]]*\{|Toggle[[:space:]]*\(|TextField[[:space:]]*\(|TextEditor[[:space:]]*\(|NavigationLink[[:space:]]*\(|Picker[[:space:]]*\(|Slider[[:space:]]*\(|PrimaryButton[[:space:]]*\(|headerIconButton[[:space:]]*\()'

# 检查窗口（行数）：交互元素行起向后扫描 N 行内是否含 accessibilityIdentifier(.
# 选 80：大多 Button/TextField body + modifier 链落在 80 行内（grid cell / wechatCard 等
# 含装饰复合 view 偏长）；选大一些减少 false positive，false negative 仅出现在巨型嵌套漏挂极端情形.
# 后续 lint 升级（如果需要更精确）可改为基于 swift-syntax 的真 AST 校验（本期不引入外部依赖）.
A11Y_WINDOW_LINES=80
# Preview 块往前扫描行数：判断当前行是否处于 Preview / PreviewProvider 块内.
PREVIEW_LOOKBACK_LINES=50

violations=0

scan_file() {
    local file="$1"
    # 抓所有交互元素行号（grep -n 输出 `line_num:content`）.
    local interactive_lines
    interactive_lines="$(grep -nE "$INTERACTIVE_PATTERN" "$file" || true)"
    if [ -z "$interactive_lines" ]; then
        return
    fi

    while IFS=: read -r line_num content; do
        [ -z "$line_num" ] && continue
        # 检查该行往后 A11Y_WINDOW_LINES 行内是否有 accessibilityIdentifier(
        local end=$((line_num + A11Y_WINDOW_LINES))
        local tail_window
        tail_window="$(awk -v start="$line_num" -v end="$end" 'NR >= start && NR <= end' "$file")"
        if echo "$tail_window" | grep -q 'accessibilityIdentifier('; then
            continue
        fi

        # 跳过 Preview 块判定：该行往前 PREVIEW_LOOKBACK_LINES 行内若含 #Preview / PreviewProvider 视为 Preview 块.
        local preview_check
        preview_check="$(awk -v end="$line_num" 'NR <= end' "$file" | tail -"$PREVIEW_LOOKBACK_LINES" | grep -E '^#Preview|PreviewProvider' || true)"
        if [ -n "$preview_check" ]; then
            continue
        fi

        echo "VIOLATION: $file:$line_num: $content" >&2
        violations=$((violations + 1))
    done <<< "$interactive_lines"
}

for dir in "${SCAN_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        continue
    fi
    while IFS= read -r -d '' file; do
        scan_file "$file"
    done < <(find "$dir" -name "*.swift" -type f -print0)
done

if [ "$violations" -gt 0 ]; then
    echo "" >&2
    echo "❌ Total violations: $violations" >&2
    echo "Fix: 在每个 violating Button/TextField/etc. 后加 .accessibilityIdentifier(AccessibilityID.<feature>.<element>)" >&2
    exit 1
fi

echo "✅ a11y coverage OK"
