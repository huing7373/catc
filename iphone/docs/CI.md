# iPhone App CI / 本地测试入口

## 1. 本地与 CI 统一入口

```bash
bash iphone/scripts/build.sh             # build only（含 xcodegen generate）
bash iphone/scripts/build.sh --test      # build + 单元测试
bash iphone/scripts/build.sh --uitest    # build + UI 测试（XCUITest）
bash iphone/scripts/build.sh --test --uitest --coverage-export   # 全跑 + 导出 coverage
bash iphone/scripts/build.sh --clean --test                       # 清干净再跑测试
```

**契约来源**：

- ADR-0002 §3.4 "CI 跑法"：本地与 CI 用同一 wrapper
- 与 server 端 `bash scripts/build.sh --test` 风格对齐（CLAUDE.md §Build & Test）

**artifacts 路径**：

- `iphone/build/test-results.xcresult` — 单元测试结果包（含 coverage 数据 / simulator 日志）
- `iphone/build/test-results-ui.xcresult` — UI 测试结果包
- `iphone/build/coverage.json` — `--coverage-export` 时产出（xcrun xccov 导出）
- `iphone/build/DerivedData/` — Xcode 增量编译缓存

## 2. Destination 三段 fallback

`build.sh` 自动 resolve destination：

1. `platform=iOS Simulator,name=iPhone 17,OS=latest`（首选；Xcode 26 默认）
2. `platform=iOS Simulator,OS=latest`（任意 iOS Simulator；Xcode 16 兼容）
3. `platform=iOS Simulator,id=<UUID>`（xcrun simctl 取第一个可用）

详见 ADR-0002 §3.4 已知坑第 2 条 P1 fix。

## 3. 未来 GitHub Actions 接入点（占位章节）

**当前状态**：未实装真 CI workflow YAML（重启阶段 + 单开发者无 CI runner）。

**未来接入时**（任意时点触发，至少节点 1 后），按以下模板写 `.github/workflows/iphone-ci.yml`：

```yaml
# 仅作示例草图，真接入时按当时 GitHub Actions 最新约定调整
name: iPhone CI
on: [push, pull_request]
jobs:
  build:
    runs-on: macos-14   # 最低 Xcode 16；macos-latest 可能滚动到 Xcode 26+
    steps:
      - uses: actions/checkout@v4
      - name: Install xcodegen
        run: brew install xcodegen
      - name: Build + Test
        run: bash iphone/scripts/build.sh --test --coverage-export
      - uses: actions/upload-artifact@v4
        with:
          name: iphone-test-results
          path: iphone/build/test-results.xcresult
```

**真接入时的 spike 内容**（不属本 story scope；归 Epic 3 Story 3.3 或更晚 spike）：

- runner 选型（macos-14 vs macos-latest 取舍 — Xcode 版本浮动）
- cache 策略（DerivedData / SPM 包是否 cache）
- artifact upload 策略（.xcresult 体积大 / 过期清理）
- 双端 CI 编排（server `bash scripts/build.sh --test` + iphone `bash iphone/scripts/build.sh --test` 是否同 job）
- destination fallback 在 CI runner 上的实际机型分布（不同 macos runner 默认 simulator 不同）

详见 ADR-0002 §1.1 "兼容性说明（单开发者重启阶段决策）" + §6 TODO "多人协作 / CI 兼容矩阵 spike"。

## 4. 排错手册

| 症状 | 可能原因 | 处置 |
|---|---|---|
| `xcodegen: command not found` | brew 未装 xcodegen | `brew install xcodegen` |
| `xcrun: error: unable to find utility "simctl"` | Xcode Command Line Tools 未装 | `xcode-select --install` |
| `iPhone 17` destination 失败 | Xcode 16 等旧版本默认机型不含 iPhone 17 | build.sh 会自动 fallback 到 `OS=latest` 或 `xcrun simctl` UUID；查看 `=== resolved destination: ... ===` 输出确认实际用的 |
| 测试在 Simulator 上首次启动慢 | Simulator 冷启动 + Xcode 编译缓存未热 | 第二次跑会快；CI 上加 `actions/cache` 保 DerivedData |
| `BUILD SUCCESS` 但实际有失败 | 没有 `set -euo pipefail` 或 fast-fail 路径绕过 | 不应出现；如出现 grep `FAIL:` 看实际位置 |

## 5. 与既有 server CI 命令面对照

| 端 | 入口 | scope |
|---|---|---|
| server | `bash scripts/build.sh --test` | Go 单测 + race + coverage |
| iPhone | `bash iphone/scripts/build.sh --test` | XCTest 单测 + coverage |
| iPhone (UI) | `bash iphone/scripts/build.sh --uitest` | XCUITest |

`bash scripts/build.sh --test`（server）与 `bash iphone/scripts/build.sh --test`（iPhone）**互不干扰**：

- artifacts 路径隔离（`build/` vs `iphone/build/`）
- 工具栈完全独立（go vs xcodebuild）
- 跨端 dev 切换零认知摩擦（同 `--test` 语义）
