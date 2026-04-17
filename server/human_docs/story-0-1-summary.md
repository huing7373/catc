# Story 0-1: 项目骨架与开发工具链 — 实现总结

Story 0-1 搭建了整个后端项目的骨架和开发工具链，没有任何业务逻辑，纯粹是"地基"。

## 做了什么

### Go 模块 + 目录结构

- 初始化了 `github.com/huing/cat/server` 模块（Go 1.24）
- 创建了分层架构的完整目录：`cmd/cat/`（入口）、`internal/` 下 10 个业务包（config、domain、service、repository、handler、dto、middleware、ws、cron、push）、`pkg/` 下 7 个工具库包（logx、mongox、redisx、jwtx、ids、fsm、clockx）
- 每个包放了一个 `doc.go` 占位文件，让 Go 工具链能识别这些包
- `cmd/cat/main.go` 只有一个空的 `func main() {}`

### 配置骨架

- `config/default.toml` 和 `config/local.toml.example`：定义了 server、log、mongo、redis、jwt、apns、cdn 七个配置段，但所有值都是空的，具体值由后续 story 填入

### 本地开发基础设施

- `deploy/docker-compose.yml`：启动 MongoDB 7.0（单节点 replica set，为后续 Change Streams 和事务做准备）和 Redis 7，供本地开发用

### 代码质量工具

- `.golangci.yml`：启用 15 个 linter，其中 `forbidigo` 禁止在业务代码中使用 `fmt.Printf/Println` 和 `log.Print/Println/Printf`（强制用 zerolog）
- `.editorconfig`：Go 文件 tab 缩进
- `.lefthook.yml`：pre-commit 钩子，提交前自动跑 gofmt、goimports、go vet

### 构建和 CI

- `Makefile`：封装了 build、test、lint、docker-up/down、ci 等常用命令
- `.github/workflows/ci.yml`：GitHub Actions 流水线，按顺序跑 lint → build → test → race test
- 复用已有的 `scripts/build.sh`，编译产物输出到 `build/catserver`

## 怎么验证的

```
bash scripts/build.sh --test
```

构建成功，产出 1.5MB 的 `catserver` 二进制，`go vet ./...` 零警告。因为只有占位代码，所有包报 `[no test files]`，这是预期行为。

## 后续 story 怎么用这个骨架

- **Story 0-2** 会在 `main.go` 里加入真正的启动逻辑（config 加载 → DI 装配 → Runnable 生命周期）
- **Story 0-3** 会在 `pkg/mongox`、`pkg/redisx` 里实现真正的数据库连接
- **Story 0-5** 会在 `pkg/logx` 里实现 zerolog 结构化日志

每个后续 story 都是往这个骨架的对应目录里填入具体实现。
