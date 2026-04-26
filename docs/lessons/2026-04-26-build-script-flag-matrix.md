---
date: 2026-04-26
source_review: codex review round 1, /tmp/epic-loop-review-2-7-r1.md
story: 2-7-ios-测试基础设施搭建
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — Shell 包装脚本的 flag 组合矩阵必须显式枚举 + 默认行为按主路径选

## 背景

Story 2.7 实装 `iphone/scripts/build.sh` 作为 iPhone 端的 build / test wrapper，提供 `--test` / `--uitest` / `--clean` / `--coverage-export` 四个 flag。codex round 1 发现 `--uitest --coverage-export` 组合下 coverage 导出步骤永远失败：UI test 写到 `test-results-ui.xcresult`，但 `xcrun xccov` 调用却硬编码读 `test-results.xcresult`（unit-only 的路径），等于这个明面上支持的 flag 组合实际上不可用。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `--uitest --coverage-export` 永远从错误的 .xcresult 读 coverage | high | config | fix | `iphone/scripts/build.sh:183-187` |

## Lesson 1: shell 包装脚本支持 N 个 flag → 必须把 2^N 组合的语义写明 + 让"主路径"决定默认数据源

- **Severity**: high
- **Category**: config（脚本 / 构建配置）
- **分诊**: fix
- **位置**: `iphone/scripts/build.sh:183-187`

### 症状（Symptom）

`bash iphone/scripts/build.sh --uitest --coverage-export` preflight 通过（"`--coverage-export` 要求 `--test` 或 `--uitest`" 这一条 OK），UI 测试跑成功，但收尾的 coverage 导出步骤 `xcrun xccov view --report --json "$TEST_RESULTS"` 读了 `iphone/build/test-results.xcresult`（unit bundle），而 UI test 写到的是 `test-results-ui.xcresult`。该 unit bundle 不存在 → xccov 报错 → 整个脚本 exit 1。等于这个组合"明面上支持，实际上 100% 失败"。

### 根因（Root cause）

写脚本时把"哪个 flag 决定输出路径"和"哪个 flag 决定数据源"两个语义耦合在变量上：

- `TEST_RESULTS="$OUTPUT_DIR/test-results.xcresult"` 这一行被多处复用 ——
  - `--test` 把它当**输出**写
  - `--uitest` 写到的是**派生路径** `${TEST_RESULTS%.xcresult}-ui.xcresult`
  - `--coverage-export` 把 `$TEST_RESULTS` 当**输入**读

`--test` 路径下三处一致，没问题。`--uitest` 路径下输出走派生路径，但 coverage 还是读原变量 → 永远 miss。

更深层的思维漏洞：写 shell 脚本时容易**只测主路径（`--test --coverage-export`）**，遇到 N 个独立 flag（这里 N=4）时不展开 2^N 组合矩阵心算"每个组合是否各自自洽"。preflight 那条"`--coverage-export` 要求 `--test` 或 `--uitest`"看似已经守住，实则只检了"非空"，没检"匹配"。

### 修复（Fix）

让 coverage 数据源**根据已运行的 test 类型动态选择**，主路径优先（unit 覆盖 production code 是测试金字塔主体）：

```diff
+ COVERAGE_SOURCE=""
+ if [ "$RUN_TESTS" = true ]; then
+   COVERAGE_SOURCE="$TEST_RESULTS"
+ elif [ "$RUN_UITESTS" = true ]; then
+   COVERAGE_SOURCE="${TEST_RESULTS%.xcresult}-ui.xcresult"
+ fi
- xcrun xccov view --report --json "$TEST_RESULTS" > "$COVERAGE_JSON"
+ xcrun xccov view --report --json "$COVERAGE_SOURCE" > "$COVERAGE_JSON"
```

规则矩阵（写在脚本注释里供后人查）：

| 组合 | coverage 源 | 备注 |
|---|---|---|
| `--test` 单独 | unit bundle | 主路径 |
| `--uitest` 单独 | UI bundle | 修复前永远 fail |
| `--test --uitest` | unit bundle | 主路径优先（unit 覆盖 prod code 更主要） |
| 都没 + `--coverage-export` | preflight 拒（已存在的检查） | exit 1 |

验证：`bash iphone/scripts/build.sh --uitest --coverage-export` 现在产出有效 `iphone/build/coverage.json`（133KB，4 个 target 的 line coverage 数据齐全）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写有 N 个独立 boolean flag 的 shell 包装脚本**时，**必须**先写出 **2^N 组合表**枚举每个组合的"输入路径 / 输出路径 / 错误信息"是否自洽，再开始实装。

> **展开**：
> - **flag 组合矩阵**写在脚本顶部注释，作为契约文档；至少枚举所有 `--xxx --yyy` 的双 flag 组合
> - **数据源选择**永远显式：不要把"输出路径变量"复用作"输入路径变量"；如果不同 flag 产生不同输出，coverage / report / artifact 这类**消费下游**必须显式 dispatch（`if/elif` 选源），不能假设"反正只有一份"
> - **多路径合一**时定主路径：当 N 个 flag 同时打开有冲突时（这里 `--test --uitest` 都开），明示哪一条是"主路径"（这里是 unit），并把这个决策写进脚本注释而不是埋在代码里
> - **preflight 校验要"匹配"，不是"非空"**：「`--coverage-export` 要求 `--test` OR `--uitest`」是非空式校验；要补一条更强的：每个 `--coverage-export` 出口路径都要能找到对应的输入 `.xcresult`。能用静态枚举消灭的运行时分支不要留
> - **dogfood 验证**：脚本写完跑**所有 flag 组合**至少一次（不只是开发时心目中的"主路径"）；本 story 自己的 wrapper 一定要 dogfood，否则 review 一定能挑出"明面支持实际不可用"的组合
> - **反例**：本次提交前 `iphone/scripts/build.sh` 只测过 `--test` / `--test --coverage-export` / `--uitest`（不带 export）三组合，没测 `--uitest --coverage-export`。preflight 通过 + 跑得起来 + 不报错并不等于"组合自洽"，因为脚本最后一步默默读了不存在的文件

## Meta: 本次 review 的宏观教训

shell 包装脚本是产品代码的"边缘地带"——没有 unit test 覆盖、没有类型系统、没有 IDE 补全；review 时容易扫一眼 happy path 通过就放行。这种地方的 bug 通过率高于业务代码，唯一对策是**把组合矩阵当作 review checklist**：reviewer 自己跑一遍每个组合，作者交付前自己跑一遍每个组合。
