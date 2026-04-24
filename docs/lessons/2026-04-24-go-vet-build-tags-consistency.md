---
date: 2026-04-24
source_review: manual review by user (inline review comment on scripts/build.sh:82-83)
story: 1-7-重做-scripts-build-sh
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-24 — `go vet` 必须跟随 build/test 的 build tag，保持 validation 可见文件集一致

## 背景

Story 1.7 重做 `scripts/build.sh`，新增 `--devtools` 开关（`-tags devtools` + 输出 `build/catserver-dev`）。首轮实装后 review 指出：`go vet ./...` 与 `go build $BUILD_TAGS ...` / `go test $BUILD_TAGS ...` 三段之间存在**可见文件集不一致** —— 脚本用 `-tags devtools` 构建 / 跑测，但 vet 永远只看没 tag 的那份。在 Go 里 `//go:build devtools` 是 **file-level** build constraint：无 tag 运行 vet 时该文件被**整体跳过**而非仅"跳过测试用例"。于是 vet 宣称"验证了全部源码"，实际上跳过了 `--devtools` 专属文件。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `go vet` 在 `--devtools` 模式下没带 `-tags devtools` | medium (P2) | testing | fix | `scripts/build.sh:82-83` |

## Lesson 1: 三件套命令（vet / build / test）必须共享同一个 build tag 变量

- **Severity**: medium (P2)
- **Category**: testing / build-tooling
- **分诊**: fix
- **位置**: `scripts/build.sh:82-83`

### 症状（Symptom）

`bash scripts/build.sh --devtools` 执行序列：

```
=== go vet ===           → go vet ./...                  （看不到 //go:build devtools 文件）
=== go build ===         → go build -tags devtools ...   （看得到）
=== go test ===          → go test  -tags devtools ...   （看得到）
```

脚本提供的 "validation path" 三步骤里，vet 步骤的可见文件集小于 build/test 步骤。`server/internal/app/http/devtools/buildtag_devtools.go`（含 `//go:build devtools`）在 `--devtools` 路径下被 build 和 test 处理，却被 vet 跳过。当前文件内容简单（一个 `const forceDevEnabled = true`），vet 检查不出毛病；但 Story 7.5 / 20.7 / 20.8 的业务 dev handler 若落进 `//go:build devtools` 路径，vet 盲区就会真实漏网。

### 根因（Root cause）

两个认知漏洞叠加触发：

1. **把 build tag 当成"跳过测试用例"在脑海中简化**。实际上 Go 的 file-level build tag（`//go:build` 首行 + package 声明之前的那种）是**编译单元筛选**，不是 test filter —— 无 tag 编译时该文件**整体**不加入 package，意味着 vet / build / test 都看不到。这是和"测试文件级 build tag（如 `//go:build integration`）"很像但语义不同的 pattern，容易混用。
2. **写 shell 脚本时按"步骤顺序"思考，而非按"每个步骤的可见文件集"思考**。把 `$BUILD_TAGS` 在 `go build` 上正确传入后，就假设"vet 是先前步骤、已经跑过了"，忽视 vet 本身也需要同一份 tag 参数才能看到同一份源码。

### 修复（Fix）

在 vet 命令上追加 `$BUILD_TAGS`（同 build/test 复用同一变量，无需新变量）：

```diff
-if ! go vet ./... 2>&1; then
+if ! go vet $BUILD_TAGS ./... 2>&1; then
```

`BUILD_TAGS` 在参数解析阶段根据 `--devtools` 设为 `-tags devtools` 或空串；空串时 shell word-splitting 产生零参数，`go vet ./...` 行为与旧版等价 —— 非 devtools 路径零回归。

验证：

- 无参 path `bash scripts/build.sh`：vet 命令 = `go vet ./...`（未变），exit 0
- devtools path `bash scripts/build.sh --devtools`：`bash -x` trace 显示实际命令 = `go vet -tags devtools ./...`，exit 0
- `--test` 全量回归（13 个包）继续全绿

### 预防规则（Rule for future Claude）⚡

> **一句话**：写 Shell / Makefile / CI 脚本包 `go vet` + `go build` + `go test` 三件套时，**凡是 `go build` 或 `go test` 用了 `-tags X`，同一次 invocation 里的 `go vet` 必须用同一个 `-tags X`**。把 tag 值提到单个 shell 变量（如 `$BUILD_TAGS`）供三个命令共用，不允许任一步骤遗漏。
>
> **展开**：
> - **适用范围**：任何 Go 项目的 wrapper 脚本 —— `build.sh` / `ci.sh` / `Makefile` / GitHub Actions step / GitLab job。
> - **判断触发条件**：脚本里出现"对同一 package 依次跑 vet、build、test"的结构，且其中至少一步带 `-tags`。
> - **不适用**：如果 vet 是**独立**的 lint job（不跟 build/test 共享 package 集合），可以按 lint 的自身策略选 tags —— 但同样要显式声明 tag，不要默认空串。
> - **原因再强调**：`//go:build X` 是 **file-level 编译单元筛选**，不是 test filter。无 tag 时整个文件从 package 里消失，vet / build / test **都**看不到。这与行级 `//go:build integration` 在 `_test.go` 里表现不同，容易混淆。
> - **反例 1（踩坑形态）**：
>   ```bash
>   go vet ./...                            # ← 固定无 tag
>   go build -tags "$TAGS" ...
>   go test  -tags "$TAGS" ...
>   ```
>   脚本宣称"先 vet 再 build 再 test"，但 vet 永远走 default 可见文件集；`//go:build $TAGS` 下的专属文件是 vet 盲区。
> - **反例 2（更隐蔽）**：把 vet 放在不同 Makefile target 里：
>   ```makefile
>   vet:
>   	go vet ./...
>   build:
>   	go build -tags devtools ...
>   ```
>   用户跑 `make vet build` 依然漏 vet。解法：`make vet` 同样参数化 `TAGS`，或用 matrix job 分别跑 `make vet TAGS=devtools` 和 `make vet TAGS=` 两个变体。
> - **反例 3（bash 变量裸传陷阱）**：`go vet "$BUILD_TAGS" ./...`（双引号包裹） —— 空串时会变成 `go vet '' ./...` 触发 `go vet: unknown flag` 错误。正确写法是 `go vet $BUILD_TAGS ./...` 让 word-splitting 处理空串（需要 `# shellcheck disable=SC2086` 抑制）。
> - **验证手段**：改完后用 `bash -x scripts/build.sh --devtools 2>&1 | grep 'go vet'` 看 trace 行实际命令，确认 `-tags` 参数真传进去了 —— 别只看 exit 0。

### 顺带改动

无。修复范围严格最小（单行 + 一条 shellcheck 抑制注释）。
