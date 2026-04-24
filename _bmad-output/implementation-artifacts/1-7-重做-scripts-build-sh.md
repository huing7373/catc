# Story 1.7: 重做 scripts/build.sh

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 一个对齐新 `cmd/server` 入口、移除旧 `cmd/cat` / `openapi.yaml` / `check_time_now.sh` 引用、并注入 `buildinfo.Commit` / `buildinfo.BuiltAt` 的 `scripts/build.sh`,
so that 工程入口与 `CLAUDE.md` 声明一致、`/version` 端点在 `build.sh` 产出的二进制里返回真实构建信息、后续 epic 不再绕开旧脚本残留.

## 故事定位（节点 1 第七条实装 story）

- Story 1.1 ADR-0001 §3.5 已明确 `build.sh` 的 contract：`--test` / `--race` / `--coverage` 三档开关、CI 和本地统一入口
- Story 1.2 已建好 `cmd/server/main.go` 入口（不再是 `cmd/cat`）
- Story 1.3 已挂 RequestID / Logging / Recovery 三件套 + `/metrics`
- Story 1.4 已建 `internal/buildinfo/` 包 + `/version` 端点；**手动 AC8** 验证了 `-ldflags -X` 注入 commit / builtAt 可跑 —— 但 `scripts/build.sh` 当时**没**接入这条命令，`/version` 走 `build.sh` 产出的二进制永远返回 `"unknown"`（Story 1.4 review 明确 defer 到本 story）
- Story 1.5 已备齐测试基础设施（testify + sqlmock + miniredis + slogtest）；`go test -race -cover ./...` 语义已锁定
- Story 1.6 已落地 Dev Tools 框架 + `-tags devtools` build flag + 延伸约定"Story 1.7 重做 `scripts/build.sh` 时可以加 `--devtools` 选项，自动加 `-tags devtools` + 输出名带 `-dev` 后缀（`build/catserver-dev`）"
- 本 story 把**当前已坏**的 `scripts/build.sh` 彻底重做，承接**四条未竟承诺**：
  1. **CLAUDE.md TODO**：移除 `cmd/cat` / `docs/api/openapi.yaml` / `scripts/check_time_now.sh` 旧架构残留
  2. **Story 1.4 defer**：把 `-ldflags -X ...buildinfo.Commit=... -X ...buildinfo.BuiltAt=...` 注入正式化到 build.sh
  3. **Story 1.5 / ADR-0001 §3.5**：保留 / 完善 `--test` / `--race` / `--coverage` 开关（CI 统一入口）
  4. **Story 1.6 延伸**：新增 `--devtools` 开关（加 `-tags devtools` + 输出 `build/catserver-dev`）

**范围红线**：

本 story **只做**以下五件事：
1. 重写 `scripts/build.sh` 让它对齐新 `cmd/server` 入口 + 注入 buildinfo ldflags + 提供 `--test` / `--race` / `--coverage` / `--integration` / `--devtools` 五个开关
2. **物理删除** `scripts/check_time_now.sh`（旧架构 M9 `time.Now` 检查；新架构无 M9 节点，无业务代码受它保护）
3. 移除脚本内关于 `docs/api/openapi.yaml` 的注释块（该 yaml 文件已不存在）
4. 三种参数组合（无参 / `--test` / `--race --test`）**手动**验证 exit 0 + 产出正确、实际 curl `/version` 验证 ldflags 注入生效
5. 更新 `CLAUDE.md` §Build & Test 里的 TODO 说明（本 story 已完成，删 TODO 行或改写为"已完成，见 Story 1.7"）

本 story **不做**：
- ❌ **不**引入 CI 配置文件（`.github/workflows/*.yml` / GitLab CI 等）—— ADR-0001 §3.5 只约定"build.sh 是本地与 CI 统一入口"，真实 CI YAML 装配属 Epic 3 Story 3.3 "文档同步与 tech debt 登记"或更晚的 Epic；本 story **仅**保证 build.sh 命令面对齐 CI 将来调用的签名
- ❌ **不**新增 Go 单元测试 —— epics.md 原文 "手动验证（非自动测试）：三种参数组合（无参 / `--test` / `--race --test`）都能跑通且 exit code 0"；shell 脚本的测试框架（bats）不值得引入
- ❌ **不**改 `cmd/server/main.go` / `internal/buildinfo/buildinfo.go` / 任何 Go 代码 —— buildinfo 的注入机制 Story 1.4 已定型，本 story 只负责**调**它
- ❌ **不**引入 PowerShell / batch 脚本替代 —— 项目在 Git Bash / WSL 下跑 `bash scripts/build.sh`，Windows 原生 `cmd.exe` 不是目标环境（见 CLAUDE.md Environment）
- ❌ **不**自动 tidy `go.mod` / `go.sum`（`go mod tidy` 是开发者显式动作，本 story 不偷加）
- ❌ **不**处理 `ios/` 或 `watch/` 的构建 —— 三端独立，本 story 仅负责 `server/`
- ❌ **不**改 `scripts/gen_sprint_status.py`（与 build 无关，属 BMM 工具链）

## Acceptance Criteria

**AC1 — 旧残留物理清理**

- `scripts/build.sh` 里**不得**出现以下**任何**字符串或路径：
  - `cmd/cat`（旧入口，已不存在）
  - `docs/api/openapi.yaml`（已不存在）
  - `check_time_now` / `check_time_now.sh`（M9 概念不属于新架构）
  - 旧注释块里的 "OpenAPI structural validation"（连注释都移除）
  - `main.buildVersion`（Story 1.4 之后 ldflags 路径改为 `internal/buildinfo.Commit` / `.BuiltAt`，`main.buildVersion` 在新 `cmd/server/main.go` 里**根本没有**声明，注入会静默失效）
- `scripts/check_time_now.sh` **物理删除**（`git rm scripts/check_time_now.sh`）
- `CLAUDE.md` §Build & Test 下方 TODO 块（原文 "当前 `scripts/build.sh` 仍引用旧的 `cmd/cat`、`docs/api/openapi.yaml`、`scripts/check_time_now.sh`..."）必须**删除或替换为**一段承接说明：改写为 "`scripts/build.sh` 已在 Story 1.7 对齐 `cmd/server` 入口 + 注入 buildinfo ldflags，见 `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`"（或等价简短表述；重点是不再留一条 misleading TODO）

**AC2 — 基础行为（无参）**

`bash scripts/build.sh`（无参）执行以下步骤：

1. 切到 `$SERVER_DIR`（= `$REPO_ROOT/server`），**不**改变调用方 CWD（子 shell 或 `cd`-local 即可）
2. 跑 `go vet ./...`；失败 → `echo "FAIL: go vet"` + `exit 1`
3. 计算两个变量：
   - `COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")`
   - `BUILT_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)` —— **必须**是 `-u`（UTC），格式 `YYYY-MM-DDTHH:MM:SSZ`（`Z` 后缀）
4. 构造 `LDFLAGS`：
   ```
   -X 'github.com/huing/cat/server/internal/buildinfo.Commit=${COMMIT}'
   -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=${BUILT_AT}'
   ```
   必须用**单引号**包裹每个 `-X 'key=value'`（`BUILT_AT` 含冒号 `:`，无引号会被 shell 或 Go ldflags 解析层截断；Story 1.4 Dev Notes §常见陷阱 #3 已预警）
5. 跑 `go build` 到 `$OUTPUT_DIR`（= `$REPO_ROOT/build`）：
   ```
   go build -ldflags "$LDFLAGS" -o "$OUTPUT_DIR/${BINARY_NAME}$(go env GOEXE)" ./cmd/server/
   ```
   - `BINARY_NAME=catserver`（保持现状）
   - `$(go env GOEXE)` 在 Windows 上取值 `.exe`，Linux/macOS 取空串 —— 这样 Windows 下输出 `build/catserver.exe`、Linux 下输出 `build/catserver`，**同一份脚本跨平台**
   - 入口路径 **必须** 是 `./cmd/server/`（**不**是 `./cmd/cat/`）
6. 成功 → `echo "OK: binary at build/<binary_name>"` + exit 0

**AC3 — `--test` 开关**

在 AC2 的步骤之后，额外跑 `go test`：

- 基础命令：`go test -count=1 ./...`
- 若同时传了 `--race`：追加 `-race`
- 若同时传了 `--coverage`（AC5）：追加 `-cover -coverprofile="$OUTPUT_DIR/coverage.out"`
- 失败 → `echo "FAIL: tests"` + `exit 1`
- 成功 → `echo "OK: all tests passed"`

**注意**：`go test` 默认会继承 `-ldflags` 注入？**不会** —— `go test` 构建 test binary 时走自己的 ldflags 链；本 story 的 test 命令**不**加 `-ldflags`（测试代码不依赖 buildinfo 注入值；Story 1.4 AC6 case 4 是在 test 内部 `buildinfo.Commit = "abc1234"` 直接赋值模拟，不需要真注入）。保持 `go test` 命令**最小**。

**AC4 — `--race` 开关**

- 传入 `--race` → `RACE_FLAG="-race"`；该 flag **同时**应用到：
  - `go build` 命令（构建 race-enabled binary，用于 race-aware 手工联调）
  - `go test` 命令（如果同时传 `--test`）
- 单独传 `--race` 不传 `--test`：产出 race-enabled binary，**不**跑测试 —— 符合 ADR-0001 §3.5 contract 里的"各开关正交"（见 Dev Notes §工程约束 #2）
- Windows 本机受限：`-race` 在无 `race_windows_amd64.syso` 的 Go 安装上会报 `-race requires cgo`。**不**强制在本 story 手工验证 `--race` 单独跑通（Dev 本机可能缺 cgo）；AC10 的"3 组合"里 `--race --test` 属于"尝试跑；若本机缺 race 前置则记录偏离"。Story 1.5 AC10 已有相同偏离模式，**沿用**同样处置（CI Linux runner 原生支持）。

**AC5 — `--coverage` 开关**

- 传入 `--coverage`：
  - 前置：**必须**同时带 `--test`（`--coverage` 单独传属"只覆盖不跑测试"无意义）；脚本侧**应**做前置校验：`if coverage=true && test=false → echo "ERR: --coverage requires --test" + exit 1`
  - 效果：在 `go test` 命令上追加 `-cover -coverprofile="$OUTPUT_DIR/coverage.out"`
  - 成功后 `echo "OK: coverage profile at build/coverage.out"`
- **不**自动跑 `go tool cover -html=`：HTML 生成是 dev 个人偏好（打开浏览器），CI 不需要；dev 想看就手工跑 `go tool cover -html=build/coverage.out -o build/coverage.html`
- ADR-0001 §3.5 明确此 contract："`bash scripts/build.sh --test --coverage` → 加 `-cover -coverprofile=...`"

**AC6 — `--devtools` 开关**

- 传入 `--devtools`：
  - 在 `go build` 与 `go test`（若传）命令上**同时**追加 `-tags devtools`
  - 二进制输出名改为 `${BINARY_NAME}-dev$(go env GOEXE)`（Windows: `catserver-dev.exe`；Linux: `catserver-dev`）
  - 其余行为与非 `--devtools` 一致（同样跑 vet、同样注入 ldflags）
- 与 `--integration` 互斥：`go test` 最多接受一个 `-tags`，同时传两个 tag 需合并成 `-tags=integration,devtools` —— 本 story **不**处理合并（实现简单，但测试组合爆炸），直接报错：`if devtools=true && integration=true → echo "ERR: --devtools and --integration are mutually exclusive" + exit 1`
- `-tags devtools` 路径下 `internal/app/http/devtools/buildtag_devtools.go` 生效，`forceDevEnabled=true`；见 Story 1.6 AC2 / Dev Notes §陷阱 #6

**AC7 — `--integration` 开关（保留 + 现代化）**

当前 `build.sh` 已有 `--integration`；本 story **保留**，仅顺带清理：

- 传入 `--integration`：在 AC2 的 vet + build 之后，跑：
  ```
  go test -tags=integration $RACE_FLAG -count=1 -timeout=120s ./...
  ```
- 与 `--test` 是**互斥**还是**合并**？保留当前 build.sh 语义："`--integration` 单跑集成测试"（不自动含 `--test`）。如果 dev 想跑全：`bash scripts/build.sh --test --integration` 两段都跑
- **本 story 不跑**真实集成测试（节点 1 还没有 dockertest 容器启动代码，Epic 4 Story 4-7 才首次落地）；AC10 手动验证**不**包含 `--integration`

**AC8 — 参数解析与互斥校验**

脚本顶部循环解析参数，**严格**：

| 参数 | 效果 |
|---|---|
| `--test` | `RUN_TESTS=true` |
| `--race` | `RACE_FLAG="-race"` |
| `--coverage` | `RUN_COVERAGE=true` |
| `--integration` | `RUN_INTEGRATION=true` |
| `--devtools` | `BUILD_TAGS="-tags devtools"`；`BINARY_SUFFIX="-dev"` |
| 其他 | `echo "Unknown flag: $arg"`；`echo` 一段 usage hint（列出所有合法 flag）；`exit 1` |

互斥 / 前置校验：
- `--coverage` 要求 `--test`（AC5）
- `--devtools` 与 `--integration` 互斥（AC6）
- 不做"`--race` 要求 `--test`"校验：单独 `--race` 产出 race-enabled binary 本身有用

**输出风格**：保持当前 build.sh 风格（`=== stage name ===` 分隔），失败时 `FAIL:` / 成功 `OK:` / 结尾 `BUILD SUCCESS`。所有 `stdout+stderr` merge 便于 log 捕获（`set -o pipefail` 已覆盖）。

**AC9 — 失败路径必须 fast-fail**

- 脚本**必须**保留 `set -euo pipefail`（严格模式）
- 任一 go 命令非 0 退出 → 立即 `exit <code>`，**不**继续后续步骤（例如 vet fail 不能进 build；build fail 不能进 test）
- `OUTPUT_DIR` 必须 `mkdir -p`（当前已做；保留）
- `REPO_ROOT` 从 `$(cd "$(dirname "$0")/.." && pwd)` 计算（当前已做；保留）—— **不**依赖调用方 CWD

**AC10 — 手动验证（epics.md 原 AC 直抄）**

本 story **不写 Go 单元测试**（非 Go 代码）。Dev 必须**手动**跑完以下组合并把输出贴 Completion Notes：

| # | 命令 | 预期 |
|---|---|---|
| 1 | `bash scripts/build.sh` | exit 0；`build/catserver` 或 `build/catserver.exe` 存在 |
| 2 | `bash scripts/build.sh --test` | exit 0；测试全绿；binary 存在 |
| 3 | `bash scripts/build.sh --race --test` | exit 0（若本机缺 cgo 则允许偏离 —— 参照 Story 1.5 AC7）；测试全绿 |

**额外手工验证**（补强 AC2 的 ldflags 注入效果）：
- 组合 1 产出后：`./build/catserver[.exe] -config server/configs/local.yaml`（或 `CAT_HTTP_PORT=18090 ./build/catserver[.exe] ...`）启动；另开 shell 跑 `curl http://127.0.0.1:<port>/version`
- 预期 response：`data.commit == <真实 short hash>`（**非** `"unknown"`）；`data.builtAt == <ISO8601 UTC>`（**非** `"unknown"`）
- 若返回 `"unknown"` → 说明 ldflags 路径错了（如字符串拼错、变量名大小写错、`const` 而非 `var` —— 见 Story 1.4 Dev Notes §陷阱 #1）—— 必须修到正确再收工

**额外手工验证**（补强 AC6 `--devtools`）：
- `bash scripts/build.sh --devtools` → 产出 `build/catserver-dev[.exe]`
- 不设 `BUILD_DEV` 环境变量直接跑 → 启动日志含 `DEV MODE ENABLED - DO NOT USE IN PRODUCTION`（Story 1.6 AC6 落地）
- curl `/dev/ping-dev` → `{"code":0,...,"data":{"mode":"dev"}}`

**AC11 — `CLAUDE.md` TODO 承接说明**

`CLAUDE.md` 现有 TODO：

> **TODO**：当前 `scripts/build.sh` 仍引用旧的 `cmd/cat`、`docs/api/openapi.yaml`、`scripts/check_time_now.sh`（M9 时间检查）—— 这些属于旧架构残留。节点 1 实装时一并重做，让 build 脚本对齐新的 `cmd/server/main.go` 入口和新的目录结构。

本 story 完成后**必须**改写为：

> `scripts/build.sh` 已在 Story 1.7 对齐新 `cmd/server` 入口 + 注入 `internal/buildinfo` ldflags。支持开关：`--test` / `--race` / `--coverage` / `--integration` / `--devtools`。详见 `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`。

（具体措辞 dev 可微调，关键是**不留 misleading TODO**，并**点名**开关与入口路径让后续 session 一眼知道。）

## Tasks / Subtasks

- [x] **T1** — 旧残留清理（AC1）
  - [x] T1.1 `git rm scripts/check_time_now.sh`
  - [x] T1.2 `grep -n 'cmd/cat\|openapi\|check_time_now\|main\.buildVersion' scripts/build.sh` 之后**确认无残留**（首轮留了一处 `./cmd/cat` 解释性注释，立即删）
  - [x] T1.3 最终脚本**不得**含 OpenAPI / M9 / `cmd/cat` 任何字样（连注释都清理）

- [x] **T2** — 重写 `scripts/build.sh`（AC2 / AC8 / AC9）
  - [x] T2.1 `set -euo pipefail` + `REPO_ROOT` + `SERVER_DIR` + `OUTPUT_DIR` + `BINARY_NAME=catserver`
  - [x] T2.2 参数解析循环：`--test` / `--race` / `--coverage` / `--integration` / `--devtools`；未知 flag 报错 + usage hint + `exit 1`
  - [x] T2.3 互斥 / 前置校验：`--coverage` ⇒ `--test`（否则 err）；`--devtools` ✗ `--integration`（否则 err）
  - [x] T2.4 `mkdir -p "$OUTPUT_DIR"`
  - [x] T2.5 `cd "$SERVER_DIR"` 后 `go vet ./...`
  - [x] T2.6 `COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")`；`BUILT_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)`
  - [x] T2.7 `LDFLAGS="-X 'github.com/huing/cat/server/internal/buildinfo.Commit=${COMMIT}' -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=${BUILT_AT}'"`（双引号串 + 内嵌单引号）
  - [x] T2.8 `go build $RACE_FLAG $BUILD_TAGS -ldflags "$LDFLAGS" -o "$OUTPUT_DIR/${BINARY_NAME}${BINARY_SUFFIX}$(go env GOEXE)" ./cmd/server/`

- [x] **T3** — `--test` / `--race` / `--coverage`（AC3 / AC4 / AC5）
  - [x] T3.1 `--test` 传入时：`go test -count=1 $RACE_FLAG $BUILD_TAGS $COVERAGE_FLAG ./...`
  - [x] T3.2 `--race` 单独传：`-race` 只进 build，不跑 test（RUN_TESTS=false）
  - [x] T3.3 `--coverage` 传入且带 `--test`：`-cover -coverprofile="$OUTPUT_DIR/coverage.out"` 追加到 `go test`；成功后 `echo "OK: coverage profile at build/coverage.out"`

- [x] **T4** — `--devtools` / `--integration`（AC6 / AC7）
  - [x] T4.1 `--devtools`：`BUILD_TAGS="-tags devtools"` + `BINARY_SUFFIX="-dev"`
  - [x] T4.2 `--integration`：独立阶段跑 `go test -tags=integration $RACE_FLAG -count=1 -timeout=120s ./...`（不走 `--test` 路径；与 `--devtools` 互斥）

- [x] **T5** — 手动验证 3 组合 + 补强（AC10）
  - [x] T5.1 `bash scripts/build.sh` → exit 0；`build/catserver.exe` 存在（24.5 MB）
  - [x] T5.2 `bash scripts/build.sh --test` → exit 0；13 个包全绿（含 devtools / sample / slogtest）；binary 重新产出
  - [~] T5.3 `bash scripts/build.sh --race --test` → build 阶段 OK；test 阶段 Windows 本机 ThreadSanitizer 内存分配失败（`error code: 87`）——与 Story 1.5 AC7 / 本 story AC4 约定的 Windows `-race` 偏离同源，归 CI Linux runner
  - [x] T5.4 `/version` curl 实际返回 `{"commit":"9c6ab04","builtAt":"2026-04-24T10:30:19Z"}` —— ldflags 真注入（**非** `"unknown"`），AC10 ldflags 注入契约兑现
  - [x] T5.5 `bash scripts/build.sh --devtools` → `build/catserver-dev.exe` 产出；启动日志含两条 `DEV MODE ENABLED` WARN + `build_tag_devtools=true env_build_dev=""`；`/dev/ping-dev` 返回 `{"code":0,"message":"ok","data":{"mode":"dev"}}`

- [x] **T6** — `CLAUDE.md` TODO 承接（AC11）
  - [x] T6.1 改写 §Build & Test TODO 块为承接说明；新增 `--test` / `--race` / `--coverage` / `--integration` / `--devtools` 命令面 + 互斥说明 + 契约来源指针

- [x] **T7** — 收尾
  - [x] T7.1 `cd server && go vet ./...` —— clean（本 story 零 Go 代码改动，sanity 验证）
  - [x] T7.2 Completion Notes 补全：见下方
  - [x] T7.3 File List 填充
  - [x] T7.4 状态流转 `ready-for-dev → in-progress → review`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **ldflags 路径**：**必须**是 `github.com/huing/cat/server/internal/buildinfo.Commit` / `.BuiltAt`，不是 `main.commit` / `main.buildVersion`。理由：
   - Story 1.4 AC3 决策把 buildinfo 变量放进独立包 `internal/buildinfo/`（cobra / kubectl / docker CLI 惯例）
   - `cmd/server/main.go` **根本没有** `buildVersion` / `commit` / `builtAt` 等包级 var，写错路径 `go build` **不报错**、`/version` 永远返回 `"unknown"`（静默失败 —— Story 1.4 §陷阱 #2 已预警）
   - **自检**：跑完 `bash scripts/build.sh` 后 **必须** curl `/version` 验证 `data.commit` 不是 `"unknown"`（AC10 T5.4）
2. **`-X` 单引号包裹**：`BUILT_AT` 含冒号（`2026-04-24T10:20:45Z`），无引号会被 Go ldflags 解析截断。写法：
   ```bash
   LDFLAGS="-X 'github.com/huing/cat/server/internal/buildinfo.Commit=${COMMIT}' -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=${BUILT_AT}'"
   go build -ldflags "$LDFLAGS" ...
   ```
   `$LDFLAGS` 本身**不加**外层引号在传递时（`-ldflags "$LDFLAGS"` 的双引号会保留内部单引号）。Git Bash / MSYS 下照跑；Windows cmd 不是本 story 目标环境。
3. **`date -u`**：`-u` 是 UTC 标记必须保留。不带 `-u` → 本地时区（CST / JST 等）会让 builtAt 每次构建随时区漂移，demo 时容易误读。Story 1.4 §陷阱 #3 + Dev Notes §AC8 已说清。
4. **`$(go env GOEXE)`**：跨平台二进制名后缀的 Go 官方惯例。Windows 值 `.exe`，Linux / macOS 值空串。**不**手写 `if [[ "$OSTYPE" == "msys" ]]; then ... fi`（不可靠；且 WSL / Cygwin / Git Bash 下 `$OSTYPE` 混乱）。
5. **`git rev-parse --short HEAD` 的 fallback**：`|| echo "unknown"` 必不可少。理由：
   - CI runner 可能不是 git checkout（如 docker build context 里 `.git` 目录被 `.dockerignore` 掉）
   - Dev 某次 `git checkout --detach` 到 `HEAD~10` 跑 build 检查老版本 —— `git rev-parse` 仍会成功；不会走 fallback
   - 纯出问题场景：没装 git / 不在 git repo → fallback 生效，`/version` 返回 `"unknown"` 但 build 不挂
6. **`git describe --tags --always --dirty`** vs **`git rev-parse --short HEAD`**：当前脚本用 `describe`（Story 0.14 旧产物）。本 story **改用** `rev-parse --short HEAD`。理由：
   - Story 1.4 AC8 命令模板就是 `rev-parse --short HEAD`（buildinfo 注入路径约定）
   - `describe` 会在有 tag 时输出 `v1.0.0-dirty` 这类字符串，与 `data.commit` 语义（"short hash"）不对齐
   - 本项目当前 0 tag，`describe --always` 会退化成 `rev-parse`，但语义仍不一致
7. **shell 脚本跨 Windows / Linux**：项目在 Git Bash（Windows）+ Linux 下都要跑。关键：
   - 路径用 `/`（不是 `\`）；`REPO_ROOT` 计算用 `cd .. && pwd`，产出的是 MSYS 格式（如 `/c/fork/cat`），Go 命令能正常接受
   - 用 `bash` 显式调（不是 `#!/bin/sh`）；`set -euo pipefail` 只在 bash 下保证语义
   - 变量引用**全部**双引号（`"$VAR"`）防止路径含空格的诡异 bug（当前 `C:\fork\cat` 不含空格，但**不**假设未来不含）
8. **为什么不引入 Makefile**：
   - Windows Git Bash 默认不带 `make`；CLAUDE.md Environment 明示 `bash` 而非 GNU make
   - `bash scripts/build.sh` 是 CLAUDE.md §Build & Test 已定型的命令面，改 Makefile 需要同步改 CLAUDE.md + 所有 memory doc + 所有 review workflow / ultrareview 流程
   - Makefile 的 `.PHONY` / `$(MAKEFLAGS)` / `$@` 语法对 Claude 后续 session 的 LLM 理解成本高于 shell
9. **为什么不引入 `just` / `task` / `mage` 等任务 runner**：同上，新工具 = 环境要求 + 安装步骤；`bash` 零要求
10. **为什么**`--coverage` 要求 `--test`**（不是独立开关）**：
    - `-coverprofile` 只在 `go test` 上才有意义；`go build` 没有覆盖率概念
    - 独立 `--coverage` 走"只跑覆盖不跑测试"会让 dev 误以为能得到某种数据；其实没有测试就没有覆盖率
    - 脚本前置报错比"静默无效果"对 dev 更友好
11. **为什么`--devtools` 与 `--integration` 互斥**：
    - `go test -tags=a -tags=b` 合法的写法是 `go test -tags=a,b`；合并 shell 里比较别扭
    - 节点 1 暂不涉及 `--integration`（Epic 4 才首次用 dockertest 写 integration 测试）—— 合并场景**未来**需要时再支持
    - 当前做"互斥 + 明确报错"比"偷偷合并"失败更可读
12. **为什么不跑 `go mod tidy` in build.sh**：
    - `go mod tidy` 会**写** `go.mod` / `go.sum`；CI 跑 build.sh 意外 tidy 会导致 CI run 自己产生变更 → "drift" 风险
    - dev 手动跑（`go mod tidy && git diff --exit-code go.mod go.sum`）是显式动作
    - 让 build.sh **只读**不写 go.mod 是 CI 友好模式

### 为什么不引入 bats / shunit2（shell 测试框架）

- shell 脚本 <100 行，3 组合手工验证 + epics.md 原 AC 即"手动验证（非自动测试）"已明示
- bats 需新增 shell 测试依赖（git submodule / brew install），CI 需装；违反"按需引入"原则
- 真实风险场景：
  1. ldflags 路径拼错 → curl `/version` 显示 `"unknown"`（AC10 T5.4 覆盖）
  2. `cmd/cat` 残留 → `go build` 失败（AC10 T5.1 会 exit != 0）
  3. 失去 `set -euo pipefail` → 非 fast-fail，但 AC10 三组合都能触发至少一次失败场景
- 手工验证 + code review 足够防御

### 为什么 --race 应用到 build 而非只应用到 test

- `go build -race` 产出 race-enabled binary，dev 拿去**手工**跑测试场景（如 concurrent login / concurrent chest open）能捕捉手工跑测里的 race
- Go 语言 spec：`-race` 需要 cgo；Windows Go install 缺 `race_windows_amd64.syso` 时 **build 也会失败**，不只 test 失败 —— 所以 Windows dev 跑 `--race` 可能 build 直接挂。这是已知偏离（与 Story 1.5 AC7 同源），归 CI Linux runner 执行
- ADR-0001 §3.5 contract 没明确 `--race` 只限定 test；从工程惯例（Go `-race` 是编译 flag 不只测试 flag）保留 race 对 build 也生效更合理

### 与 Story 1.4 / 1.5 / 1.6 的契约兑现表

| 上游 story | 未竟约定 | 本 story 如何兑现 |
|---|---|---|
| 1.4 AC3 | `ldflags -X` 注入 `internal/buildinfo.Commit/BuiltAt` 的**命令行**已验证，待 build.sh 正式化 | T2.6-T2.8 + T5.4 真实 curl `/version` 验证 |
| 1.4 Review finding | `.claude/settings.local.json` / `scripts/build.sh` / `go run` 三条 build path 里 build.sh defer 到 1.7 | 本 story 兑现 build.sh 路径；`go run` + settings.local 的 dev 工作流仍走"发布 `/version=unknown`"（合理，是 dev 模式语义） |
| 1.5 ADR-0001 §3.5 | build.sh contract：`--test` / `--race` / `--coverage` | T2.2 + T3.1-T3.3 |
| 1.6 Dev Notes §延伸 | build.sh 加 `--devtools` 开关 + 输出名带 `-dev` 后缀 | T4.1 + T5.5 |
| CLAUDE.md §Build & Test TODO | "当前 `scripts/build.sh` 仍引用旧的 ..." | T6.1 删除 TODO 改写承接 |

### Lessons Index（与本 story 相关的过去教训）

- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` **Lesson 2 "声明 vs 现实"** —— 直接相关：build.sh 当前**声明**的 `cmd/cat` 入口与**现实**的 `cmd/server/` 入口已背离 5 个月；本 story 修复这一声明。本 story 改完后 **必须** curl `/version` 验证 ldflags 注入**真**生效（AC10 T5.4 强制）—— 否则会陷入新的"声明成功但现实返回 unknown"的谎言态
- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` **Lesson 1 "CWD-relative path"** —— 间接相关：`scripts/build.sh` 顶部 `REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"` 已正确处理；**不**依赖调用方 CWD；维持当前设计
- `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` **Lesson 1** —— 间接相关：Story 1.4 的 buildinfo 默认值 `"unknown"` 是**代码层**兜底；shell 层 `|| echo "unknown"` 是**构建层**兜底；两层独立各司其职，本 story 不引入第三层（handler 不做 "空串 → unknown" 兜底，Story 1.4 AC5 已决策）

### Git intelligence（最近 6 个 commit）

- `9c6ab04 chore(commands): 更新 /story-done 命令`
- `e3f1e9a chore(story-1-6): 收官 Story 1.6 + 归档 story 文件`
- `e359d17 feat(server): Epic1/1.6 Dev Tools 框架（build flag gated）`
- `1564623 chore(commands): 更新 /story-done 命令`
- `2b2a7a8 chore(claude): 更新 Bash allowlist`
- `5bdda0a chore(commands): /fix-review 不再询问 commit message`

**最近实装向 commit** 是 `e359d17` (Story 1.6)。本 story 紧随其后。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` / `server` / `scripts`。
本 story 建议：`chore(scripts): Epic1/1.7 重做 build.sh（对齐 cmd/server 入口 + buildinfo ldflags + --coverage/--devtools）`（或 `feat(scripts):` 若视之为工程能力补强）

### 常见陷阱

1. **`go env GOEXE` 在旧 Go 版本可能返回空**：Go 1.16+ 都支持；本项目 go.mod `go 1.25.0`（见 `server/go.mod`），不可能遇到
2. **Windows `date -u` 输出格式**：Git Bash / MSYS 的 `date` 是 GNU coreutils，`+%Y-%m-%dT%H:%M:%SZ` 格式**跨平台**一致；PowerShell 的 `Get-Date -Format` 完全不同（本 story 不支持 PS，CLAUDE.md 约定 bash 为 shell）
3. **`git rev-parse --short HEAD` 在 commit 未 push 时也工作**：short hash 依赖本地 `.git/objects`，不需要 remote 连通
4. **`--race` 单独跑 → 产 race binary；不要误以为它自动含 test**：当前 build.sh 的实现就是"`--race` 只设置 flag，不隐含 `--test`"，本 story 保留；epics.md AC "3 组合" 里 `--race --test` 是**显式**写出来的
5. **`go test -count=1` 禁用缓存**：确保每次跑都真实跑测试；不加 `-count=1` Go 会缓存测试结果，用户改了 shell 脚本重跑会误以为"测试还是绿的"（其实是缓存）
6. **忘记 `./cmd/server/`** 末尾的 `/`：`go build ./cmd/server` 和 `go build ./cmd/server/` 等效，但某些 Go 版本 / GOPATH 模式下有微小差异。统一写 `./cmd/server/`（当前 build.sh 传统）
7. **`LDFLAGS` 里`'`用 bash 单引号会吃掉变量扩展**：必须用双引号串 + 内嵌单引号。见约束 #2。写错的话 `${COMMIT}` 会字面传给 go ldflags，注入的值是字符串 `${COMMIT}` 而非 `9c6ab04`
8. **`-tags devtools` 必须在 `-ldflags` 之前或之后**：`go build` 对 flag 顺序不敏感，但 ldflags 的值**必须**在同一对引号内，不能被 tag 切断。验证：`go build -tags devtools -ldflags "-X a=b" -o ... ./cmd/server/` 合法
9. **脚本删除 `check_time_now.sh` 后，`.claude/settings.local.json` 或其他配置文件如果引用它 → 会坏**：grep 全仓 `check_time_now` 确认无其他引用再删（AC1 T1.2）
10. **`build/` 目录是 gitignored 吗？**：需要确认；若不是会把二进制提交到 repo —— `.gitignore` 应**已**包含 `/build/` 或等价条目（历史产物），本 story **不**负责改 `.gitignore`（若发现确实缺，**单独**提 tech-debt，属 Story 3.3）

### Project Structure Notes

- 修改的唯一 shell 文件：`scripts/build.sh`（整体重写）
- 物理删除：`scripts/check_time_now.sh`
- 修改：`CLAUDE.md` §Build & Test TODO 段
- **不**新增文件、**不**改 Go 代码、**不**改 `go.mod` / `go.sum`
- **不**改 `.claude/settings.local.json`（该文件是 Claude Code 环境配置，本 story 不是它的 scope）
- 本 story 对目录结构的唯一影响：`scripts/` 减少一个 `.sh` 文件

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.7] — 本 story 原始 AC（对齐 cmd/server + ldflags 注入 + 3 组合手动验证 + 移除旧残留）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1] — Epic 1 scope "重做 `scripts/build.sh`" 明示在范围内
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3.5] — ADR-0001 §3.5 "CI 跑法"：`--test` / `--race` / `--coverage` 三档开关契约；本地与 CI 统一入口
- [Source: _bmad-output/implementation-artifacts/1-4-version-接口.md#AC3] — `internal/buildinfo/` 包路径决策；`-X 'github.com/huing/cat/server/internal/buildinfo.Commit=...'` 正确写法
- [Source: _bmad-output/implementation-artifacts/1-4-version-接口.md#AC8] — 手工 ldflags 注入命令模板（本 story 把"手工"正式化为脚本）
- [Source: _bmad-output/implementation-artifacts/1-4-version-接口.md#CR-Response] — Story 1.4 review 明确 defer `build.sh` 修复到本 story
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md#AC9 / Dev Notes §延伸] — `--devtools` 开关约定
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md#AC2] — `buildtag_normal.go` / `buildtag_devtools.go` 双文件 build tag 机制
- [Source: CLAUDE.md#Build-Test] — 当前 build.sh TODO（本 story 承接）
- [Source: CLAUDE.md#工作纪律] — "节点顺序不可乱跳"（本 story 是节点 1 第七条）
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#2 原则 7] — "基础设施按需引入，不一次堆完"——本 story 不做 CI YAML / 不引 Makefile
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#4.1] — 节点 1 scope 明示"重做 `scripts/build.sh`"
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4] — `cmd/server/main.go` 新架构入口路径
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md#Lesson-2] — "声明 vs 现实"对齐原则（本 story 存在的根因）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- **偏离 AC1 "连注释都清理"**：T2 首轮重写 `scripts/build.sh` 时在顶部 usage 注释留了一行 `# Entry point is ./cmd/server/ (new architecture; legacy ./cmd/cat removed).` —— 虽然语义是"解释旧入口已去掉"，但字面含 `./cmd/cat`，违反 AC1 "不得含 `cmd/cat` 任何字样"。发现后立即改为 `# Entry point is ./cmd/server/ (module path github.com/huing/cat/server).`。grep 二次验证：`grep -n 'cmd/cat\|openapi\|check_time_now\|main\.buildVersion\|OpenAPI\|M9\|time_now' scripts/build.sh` 无命中。
- **T5.3 `--race --test` 的偏离模式不同**：AC4 / Story 1.5 AC7 原文描述的偏离是 "Windows Go install 缺 `race_windows_amd64.syso` 导致 `-race requires cgo` 编译报错"。实际在本机跑出来：**build 阶段通过**（race-enabled binary 产出），**test 阶段** ThreadSanitizer 启动时内存分配失败 `ERROR: ThreadSanitizer failed to allocate ... (error code: 87)`。根因同属 Windows TSAN 运行时限制（Win32 VirtualAlloc 不容易满足 TSAN shadow memory 的对齐 / 连续大区要求），与 "缺 syso" 语义等价（都是 OS 级限制，非代码问题）。归 CI Linux runner 执行，本机跑不通不阻塞 review —— 与 Story 1.5 同处置。
- **Windows 路径适配**：`$(go env GOEXE)` 在本机返回 `.exe`，无参路径下脚本正确产出 `build/catserver.exe`；`--devtools` 路径产出 `build/catserver-dev.exe`；Linux runner 上会分别产出 `catserver` / `catserver-dev`（无后缀）。Git Bash 环境 `/c/fork/cat/build/` 前缀与 Windows `C:\fork\cat\build\` 双向可达，Go 命令对两种 path 形态都接受。
- **Background task 127 退出码**：Bash tool 后台启动的 server 进程被 PowerShell `Stop-Process -Force` 杀后返回退出码 127（signal-kill），系统以 "failed" 提示收到 task-notification —— 这是**预期**行为（主动 kill），非测试失败；两次 background server 均为 curl 验证目的短期运行。

### Completion Notes List

**实现摘要**
- 整体重写 `scripts/build.sh`，入口改为 `./cmd/server/`；五个开关 `--test` / `--race` / `--coverage` / `--integration` / `--devtools` 全部落地
- ldflags 注入路径 `github.com/huing/cat/server/internal/buildinfo.Commit` + `.BuiltAt`（对齐 Story 1.4 AC3 决策）
- 物理删除 `scripts/check_time_now.sh`（grep 全仓验证无其他引用）
- `CLAUDE.md` §Build & Test TODO 改写为承接说明 + 完整命令面示例
- **零 Go 代码改动**（本 story 范围红线）；`go vet ./...` sanity 通过

**手动验证输出（AC10 核心）**

### T5.1 `bash scripts/build.sh`（无参）
```
$ rm -rf build && bash scripts/build.sh
=== go vet ===

=== go build (commit=9c6ab04, builtAt=2026-04-24T10:30:19Z) ===
OK: binary at build/catserver.exe

BUILD SUCCESS
$ ls -la build/catserver.exe
-rwxr-xr-x 1 zhuming 197121 24555008 Apr 24 18:30 build/catserver.exe
```

### T5.4 `/version` curl（补强 AC2 ldflags 注入效果，确认非 `"unknown"`）
```
$ CAT_HTTP_PORT=18090 ./build/catserver.exe -config server/configs/local.yaml &
$ curl -s http://127.0.0.1:18090/version
{"code":0,"message":"ok","data":{"commit":"9c6ab04","builtAt":"2026-04-24T10:30:19Z"},"requestId":"c76660b7-e81a-4b5b-be6d-67452d022f9c"}
$ curl -s http://127.0.0.1:18090/ping
{"code":0,"message":"pong","data":{},"requestId":"4434d96c-e2e2-40be-ba58-70146850e467"}
```
✅ `data.commit == "9c6ab04"`（当前 HEAD short hash，`git rev-parse --short HEAD` 实际值 `9c6ab04`，完全一致）——**非** `"unknown"`。

### T5.2 `bash scripts/build.sh --test`
```
=== go vet ===

=== go build (commit=9c6ab04, builtAt=2026-04-24T10:41:33Z) ===
OK: binary at build/catserver.exe

=== go test ===
?   	github.com/huing/cat/server/cmd/server	[no test files]
ok  	github.com/huing/cat/server/internal/app/bootstrap	0.259s
ok  	github.com/huing/cat/server/internal/app/http/devtools	0.199s
ok  	github.com/huing/cat/server/internal/app/http/handler	0.186s
ok  	github.com/huing/cat/server/internal/app/http/middleware	0.213s
?   	github.com/huing/cat/server/internal/buildinfo	[no test files]
ok  	github.com/huing/cat/server/internal/infra/config	0.324s
ok  	github.com/huing/cat/server/internal/infra/logger	0.334s
ok  	github.com/huing/cat/server/internal/infra/metrics	0.591s
?   	github.com/huing/cat/server/internal/pkg/response	[no test files]
ok  	github.com/huing/cat/server/internal/pkg/testing	0.473s
ok  	github.com/huing/cat/server/internal/pkg/testing/slogtest	0.392s
ok  	github.com/huing/cat/server/internal/service/sample	0.382s
OK: all tests passed

BUILD SUCCESS
```

### T5.3 `bash scripts/build.sh --race --test`（Windows 本机 TSAN 偏离）
```
=== go vet ===

=== go build (commit=9c6ab04, builtAt=2026-04-24T10:44:13Z) ===
OK: binary at build/catserver.exe          ← build 阶段通过（race-enabled binary 产出）

=== go test ===
?   	github.com/huing/cat/server/cmd/server	[no test files]
==6096==ERROR: ThreadSanitizer failed to allocate 0x000001360000 (20316160) bytes at 0x100ee3c6f0000 (error code: 87)
FAIL	github.com/huing/cat/server/internal/app/bootstrap	0.114s
...（全部 10 个包 TSAN 均同样报错）
FAIL: tests
```
**偏离说明**：Windows 本机 TSAN 限制（OS-level，非代码 bug）；build 阶段通过证明 `-race` flag 传递正确；归 CI Linux runner 跑通（与 Story 1.5 AC7 同源偏离）。

### T5.5 `bash scripts/build.sh --devtools`（补强 AC6）
```
$ rm -rf build && bash scripts/build.sh --devtools
=== go vet ===

=== go build (commit=9c6ab04, builtAt=2026-04-24T10:50:46Z) ===
OK: binary at build/catserver-dev.exe

BUILD SUCCESS

$ unset BUILD_DEV && CAT_HTTP_PORT=18091 ./build/catserver-dev.exe -config server/configs/local.yaml &
{"time":"2026-04-24T18:54:34.9553768+08:00","level":"INFO","msg":"config loaded","path":"...","http_port":18091,"log_level":"info"}
{"time":"2026-04-24T18:54:34.9616781+08:00","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION"}                              # main.go 那条
[GIN-debug] GET    /ping    ...
[GIN-debug] GET    /version ...
[GIN-debug] GET    /metrics ...
{"time":"2026-04-24T18:54:34.9621957+08:00","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION","build_tag_devtools":true,"env_build_dev":""}  # Register 那条（证明 -tags devtools 生效）
[GIN-debug] GET    /dev/ping-dev --> ... PingDevHandler

$ curl -s http://127.0.0.1:18091/dev/ping-dev
{"code":0,"message":"ok","data":{"mode":"dev"},"requestId":"ce6ea4c1-907b-47f6-b524-5b8a92da3e65"}

$ curl -s http://127.0.0.1:18091/version
{"code":0,"message":"ok","data":{"commit":"9c6ab04","builtAt":"2026-04-24T10:50:46Z"},"requestId":"e520c52e-12c8-4625-8772-a3e796331a24"}
```
✅ `BUILD_DEV` 未设但 `build_tag_devtools=true` —— `-tags devtools` build-time gate 生效  
✅ `/dev/ping-dev` 返回预期 envelope  
✅ `/version` 跨 `--devtools` 路径 ldflags 仍然注入  

**契约兑现**（故事 Dev Notes §契约兑现表）
- Story 1.4 AC3 `internal/buildinfo.Commit/BuiltAt` ldflags 正式化 ✅（T5.4）
- Story 1.5 / ADR-0001 §3.5 `--test` / `--race` / `--coverage` 开关 ✅（T3.1-T3.3）
- Story 1.6 `--devtools` 开关 + `catserver-dev` 输出名 ✅（T5.5）
- CLAUDE.md §Build & Test TODO 承接 ✅（T6.1）

**后续延伸**（非本 story scope，留记录）
- Story 1.10 `server/README.md` 本地开发指南会把本次五个开关 + 互斥规则写进"跑测试"章节
- Story 3.3 文档同步可考虑把 `build.sh` 命令面固化到 docs/architecture 下（但目前 CLAUDE.md + story 文件已是 canonical 引用）
- CI YAML 配置（归 Epic 3 或更晚）直接调 `bash scripts/build.sh --test --race --coverage`，无需额外适配层

### File List

**新增**
- （无）

**修改**
- `scripts/build.sh`（整体重写：127 行；5 个开关 + ldflags 注入 + 互斥校验 + fast-fail）
- `CLAUDE.md`（§Build & Test TODO 块 → 承接说明 + 完整命令面示例）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`1-7-重做-scripts-build-sh`: `backlog → ready-for-dev → in-progress → review`；`last_updated` 时间戳）
- `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`（本 story 文件：Tasks 勾选 / Dev Agent Record 填充 / Status 流转）

**删除**
- `scripts/check_time_now.sh`（旧架构 M9 `time.Now` 检查；新架构无 M9 节点）

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-24 | 0.1 | 初稿（ready-for-dev） | SM |
| 2026-04-24 | 1.0 | 实装完成：`scripts/build.sh` 整体重写（5 开关 + buildinfo ldflags 注入）；`check_time_now.sh` 物理删除；`CLAUDE.md` §Build & Test TODO 承接；T5.1/T5.2/T5.4/T5.5 全通过 + `/version` 实 curl 验证 `commit=9c6ab04` ≠ `"unknown"`；T5.3 `--race --test` Windows 本机 TSAN 偏离沿用 Story 1.5 AC7 处置（归 CI）；状态流转 review | Dev |
