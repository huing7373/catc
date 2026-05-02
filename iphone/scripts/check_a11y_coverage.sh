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

# Story 37.13 fix-review round 3 修法（codex 指出的 algorithm soundness bug）：
# 旧算法对每个 interactive line 取固定 80 行 window 检查 accessibilityIdentifier(.
# 当多个 control 在同一 view 内且首个 control 漏挂时，由于下方 sibling control 的
# identifier 在同一 80 行 window 内会被错误判 OK → CI gate bypassed.
#
# 新算法：每 interactive line 的 window 上限收紧为
# `min(line + A11Y_WINDOW_LINES, next_interactive_line - 1)`.
# 这样首个 control 的 window 在到达下一个 control 之前就截止 → 多 control 同 view 时
# 每个 control 各管自己的 body / modifier 链；首 control 漏挂不会被 sibling 顺势遮蔽.
#
# 边界 case（已知漏报，需要 swift-syntax AST 才能彻底解决）:
#   - 首个 interactive 与下个 interactive 之间夹一个由 helper 调用产生的 sibling
#     accessibilityIdentifier (例如 `.accessibilityIdentifier(...)` 出现在两个 Button
#     行之间但归属上一个 Button) → 仍可能 false negative；这种结构罕见但请人评.
#   - 同一行内多个 .accessibilityIdentifier 调用（跨多个 control）—— 不构造此类代码.
#
# 守护测试见末尾 fixture 验证（fixture 在 iphone/scripts/check_a11y_coverage.fixture/）.
scan_file() {
    local file="$1"
    # 抓所有交互元素行号（grep -n 输出 `line_num:content`）.
    local interactive_lines
    interactive_lines="$(grep -nE "$INTERACTIVE_PATTERN" "$file" || true)"
    if [ -z "$interactive_lines" ]; then
        return
    fi

    # 把所有 interactive 行号按 ascending 序写入临时文件，便于 awk 取下一个 line_num.
    local nums_file
    nums_file="$(mktemp -t a11y_nums.XXXXXX)"
    echo "$interactive_lines" | awk -F: '{print $1}' | sort -n > "$nums_file"

    while IFS=: read -r line_num content; do
        [ -z "$line_num" ] && continue
        # 跳过注释行（`//` / `///` / `/*` / `*` 起首）—— pattern 在注释里出现是误报
        # （例如 `/// - 加回 ... PrimaryButton(variant: .secondary)`）.
        local stripped
        stripped="$(printf '%s' "$content" | sed 's/^[[:space:]]*//')"
        case "$stripped" in
            '//'*|'///'*|'/*'*|'*'*) continue ;;
        esac
        # 跳过 Swift function 声明行 —— 这些行只是定义 helper 而不是 callsite，
        # 真正的 callsite 会在别处单独命中 INTERACTIVE_PATTERN 并自带检查.
        # 例：`private func headerIconButton(iconKey: ...) -> some View {`.
        case "$content" in
            *' func '*'('*) continue ;;
        esac
        # 算 window 上限：min(line_num + A11Y_WINDOW_LINES, next_interactive_line - 1).
        local cap_end=$((line_num + A11Y_WINDOW_LINES))
        local next_line
        next_line="$(awk -v cur="$line_num" '$1 > cur { print $1; exit }' "$nums_file")"
        local end
        if [ -n "$next_line" ]; then
            local natural_end=$((next_line - 1))
            if [ "$natural_end" -lt "$cap_end" ]; then
                end=$natural_end
            else
                end=$cap_end
            fi
        else
            end=$cap_end
        fi
        # window 起点也截到当前 control 自己 (line_num) 之后 — 不允许往前看（防 helper
        # callsite 的 .accessibilityIdentifier 反向覆盖到 helper 内部 Button false OK，
        # 反向覆盖路径靠 INTERACTIVE_PATTERN 把 helper 名加进 pattern 处理）.
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

    rm -f "$nums_file"
}

# --- Self-test mode -----------------------------------------------------------
# Story 37.13 r3: 守护算法 sound 性的 fixture 测试.
# 用 `bash check_a11y_coverage.sh --self-test` 触发：构造一个含「漏挂 Button +
# sibling 已挂 Button」的 fixture，跑 scan_file 应当报 1 个 violation.
# 若回归（violation 计数 ≠ 1）则脚本退出 != 0，提示算法又被 bug 污染.
if [ "${1:-}" = "--self-test" ]; then
    fixture_dir="$(mktemp -d -t a11y_selftest.XXXXXX)"
    cat > "$fixture_dir/Adversarial.swift" <<'FIXTURE'
import SwiftUI

struct AdversarialView: View {
    var body: some View {
        VStack {
            // 第 1 个 Button：故意漏挂 a11y identifier.
            Button(action: { print("first") }) {
                Text("first")
            }

            // 第 2 个 Button：挂了 a11y identifier.
            // sound 算法应当只把下面 identifier 算给第 2 个 Button，第 1 个仍报 violation.
            Button(action: { print("second") }) {
                Text("second")
            }
            .accessibilityIdentifier("second")
        }
    }
}
FIXTURE
    scan_file "$fixture_dir/Adversarial.swift"
    rm -rf "$fixture_dir"
    if [ "$violations" -eq 1 ]; then
        echo "✅ self-test passed: sound algorithm catches missing identifier on first sibling Button"
        exit 0
    else
        echo "❌ self-test FAILED: expected 1 violation, got $violations" >&2
        echo "   Algorithm regression — first-sibling-missing should NOT be masked by next-sibling identifier" >&2
        exit 1
    fi
fi
# ------------------------------------------------------------------------------

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
