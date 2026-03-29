# Story 2.1: Go 后端基础架构与数据库

Status: ready-for-dev

## Story

As a 开发者,
I want 搭建 Go 后端服务基础框架和数据库迁移系统,
So that 后续所有服务端功能有统一的基础设施。

## Acceptance Criteria

1. **Given** server/ 目录从零搭建 **When** 按照架构文档初始化 Go 后端 **Then** 项目结构包含 `cmd/server/main.go`、`internal/{config,middleware,handler,service,repository,model,dto}`、`pkg/{jwt,redis,validator}`
2. **Given** Gin 路由框架初始化 **When** 请求 GET /health **Then** 返回 200 + JSON（含 PostgreSQL/Redis 状态 + goroutine 计数 + uptime）
3. **Given** 数据库配置正确 **When** 服务启动 **Then** PostgreSQL 连接通过 GORM 建立，golang-migrate 迁移系统就绪，首个迁移（users 表）可执行
4. **Given** Redis 配置正确 **When** 服务启动 **Then** Redis 连接初始化（带密码）
5. **Given** 日志系统配置 **When** 任意 HTTP 请求到达 **Then** zerolog 结构化 JSON 日志输出到 stdout，含 timestamp/request_id/endpoint/duration_ms/status_code
6. **Given** Docker 环境 **When** 运行 docker-compose up **Then** PostgreSQL + Redis 容器启动，端口仅绑定 127.0.0.1
7. **Given** 环境配置 **When** 服务启动 **Then** 从 .env.development 加载配置

## Tasks / Subtasks

- [ ] Task 1: 初始化 Go 模块和依赖 (AC: #1)
  - [ ] 1.1 `go mod init github.com/huing7373/catc/server`
  - [ ] 1.2 安装依赖: gin, gorm, gorm/postgres, go-redis/v9, golang-jwt/jwt/v5, zerolog, robfig/cron/v3, gorilla/websocket, sideshow/apns2, testify
  - [ ] 1.3 创建完整目录结构（见下方 File Structure）
- [ ] Task 2: 配置管理 (AC: #7)
  - [ ] 2.1 实现 `internal/config/config.go` — 从环境变量/.env 文件加载配置
  - [ ] 2.2 更新 `.env.development` 配置项
- [ ] Task 3: 数据库连接与迁移 (AC: #3)
  - [ ] 3.1 GORM PostgreSQL 连接初始化
  - [ ] 3.2 golang-migrate 迁移系统集成
  - [ ] 3.3 创建首个迁移: `migrations/{timestamp}_create_users.up.sql` + `.down.sql`
  - [ ] 3.4 users 表结构: id(UUID PK), apple_id(unique), display_name, device_id, dnd_start, dnd_end, is_deleted, deletion_scheduled_at, created_at, last_active_at
- [ ] Task 4: Redis 连接 (AC: #4)
  - [ ] 4.1 实现 `pkg/redis/redis.go` — 带密码的 Redis 客户端封装
- [ ] Task 5: 日志中间件 (AC: #5)
  - [ ] 5.1 实现 `internal/middleware/logger.go` — zerolog + request_id + user_id 注入
- [ ] Task 6: 路由与 Health 端点 (AC: #2)
  - [ ] 6.1 实现 `internal/handler/health.go` — PG/Redis 状态 + goroutine 数 + uptime
  - [ ] 6.2 `cmd/server/main.go` — initDB → initServices → initRouter → Run
  - [ ] 6.3 路由: `/health` (public), API 路由组 `/v1/` 预留
- [ ] Task 7: 错误响应标准格式 (AC: #2)
  - [ ] 7.1 实现 `internal/dto/error_dto.go` — 统一 `{error: {code, message}}` 格式
  - [ ] 7.2 实现 `respondError()` 和 `respondSuccess()` 辅助函数
- [ ] Task 8: Docker Compose (AC: #6)
  - [ ] 8.1 验证/更新 `server/deploy/docker-compose.yml`（PG + Redis 绑 127.0.0.1 + 密码）
- [ ] Task 9: WebSocket 空实现 (AC: #1)
  - [ ] 9.1 创建 `internal/ws/{hub.go, client.go, room.go}` 空结构体 + 注释 "Growth 阶段实现"
- [ ] Task 10: 测试 (AC: all)
  - [ ] 10.1 Health handler 测试
  - [ ] 10.2 Config 加载测试
  - [ ] 10.3 迁移 up/down 测试

## Dev Notes

### Architecture Compliance — 严格遵循

**三层架构：Handler → Service → Repository → Model**
- Handler：仅做参数解析和响应格式化
- Service：业务逻辑，可被多个 Handler 复用
- Repository：包装 GORM 查询，Service 禁止直接操作 DB
- Repository 所有查询必须带 user_id（防 IDOR 攻击）
- Service 间调用单向 OK，双向禁止

**依赖注入：手动构造函数，零框架**
```go
// main.go 模式
func main() {
    cfg := config.Load()
    db := initDB(cfg)
    rdb := initRedis(cfg)
    // repos
    userRepo := repository.NewUserRepo(db)
    // services
    authService := service.NewAuthService(userRepo, rdb)
    // handlers
    authHandler := handler.NewAuthHandler(authService)
    // router
    r := initRouter(authHandler, ...)
    r.Run(":" + cfg.ServerPort)
}
```

### Database — 命名规范

| 规则 | 约定 | 示例 |
|------|------|------|
| 表名 | 复数 snake_case | `users`, `gift_sequences` |
| 列名 | snake_case | `user_id`, `created_at` |
| 外键 | `{引用表单数}_id` | `user_id`, `skin_id` |
| 索引 | `idx_{表}_{列}` | `idx_users_apple_id` |
| 唯一约束 | `uq_{表}_{列}` | `uq_users_apple_id` |
| 布尔列 | `is_` 前缀 | `is_active`, `is_deleted` |
| 时间列 | `_at` 后缀 | `created_at`, `last_active_at` |
| ID | String UUID | 禁止自增整数（防枚举攻击）|

### 首个迁移 — users 表

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    apple_id VARCHAR(255) NOT NULL,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    device_id VARCHAR(255) NOT NULL DEFAULT '',
    dnd_start TIME,
    dnd_end TIME,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    deletion_scheduled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_users_apple_id ON users(apple_id) WHERE is_deleted = FALSE;
CREATE INDEX idx_users_last_active ON users(last_active_at);
CREATE INDEX idx_users_deletion_scheduled ON users(deletion_scheduled_at) WHERE is_deleted = TRUE;
```

### API 规范

- 路由前缀: `/v1/`
- JSON 字段: snake_case
- 日期格式: ISO 8601 UTC `2026-03-27T10:30:00Z`
- ID: String UUID
- 成功响应: 直接返回数据，禁止 `{data: ..., success: true}` 包装
- 错误响应: `{"error": {"code": "ERROR_CODE", "message": "debug info"}}`
- 自定义 Header: `X-Cat-Device-Id`, `X-Cat-Client-Version`
- 分页: cursor-based `{items, next_cursor, has_more}`

### Go 代码规范

| 规则 | 约定 | 示例 |
|------|------|------|
| 文件名 | snake_case | `auth_service.go` |
| Struct | PascalCase | `BlindBoxService` |
| 导出方法 | PascalCase | `SyncSequence()` |
| 私有方法 | camelCase | `validateToken()` |
| 变量 | camelCase | `userID`, `skinConfig` |
| 接口 | Verb+er / Noun | `SkinRepository`, `TokenValidator` |
| 错误变量 | `Err` 前缀 | `ErrSequenceConflict` |

### 日志标准字段

zerolog JSON 输出必须包含: `timestamp`, `request_id`, `user_id`(认证后), `endpoint`, `duration_ms`, `status_code`

### Rate Limiter（本 Story 仅预留结构）

本 Story 创建 `internal/middleware/rate_limiter.go` 文件但内容为空实现/TODO。Story 2.2 认证完成后再实现具体限流逻辑。

### 测试标准

- Go Service 覆盖率 ≥ 80%
- Go Handler 覆盖率 ≥ 70%
- Go Middleware 覆盖率 ≥ 90%
- 使用 testify 断言库
- 测试文件与源文件同目录: `xxx_test.go`

### Project Structure Notes

本 Story 交付后 server/ 目录结构：

```
server/
├── cmd/server/main.go              # 入口: initDB → initServices → initRouter → Run
├── internal/
│   ├── config/config.go            # 环境变量配置加载
│   ├── middleware/
│   │   ├── logger.go               # zerolog request_id + user_id
│   │   ├── auth.go                 # 空实现（Story 2.2）
│   │   ├── rate_limiter.go         # 空实现（Story 2.2+）
│   │   └── cors.go                 # 空实现
│   ├── handler/
│   │   └── health.go               # GET /health
│   ├── service/                    # 空目录（后续 Story 填充）
│   ├── repository/
│   │   └── user_repo.go            # 基础 CRUD（配合迁移验证）
│   ├── model/
│   │   └── user.go                 # GORM User model
│   ├── dto/
│   │   └── error_dto.go            # 统一错误响应
│   └── ws/
│       ├── hub.go                  # 空实现
│       ├── client.go               # 空实现
│       └── room.go                 # 空实现
├── pkg/
│   ├── redis/redis.go              # Redis 客户端封装
│   └── jwt/jwt.go                  # 空文件（Story 2.2）
├── migrations/
│   └── {timestamp}_create_users.{up,down}.sql
├── deploy/
│   ├── docker-compose.yml
│   └── Dockerfile
├── .env.development
├── go.mod
└── go.sum
```

### References

- [Source: _bmad-output/planning-artifacts/architecture.md — Server Directory Structure (lines 179-236, 925-1005)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Handler-Service-Repository (lines 269-278, 1050-1057)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Database Conventions (lines 576-587)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Go Code Conventions (lines 602-612)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Error Handling (lines 425-454)]
- [Source: _bmad-output/planning-artifacts/architecture.md — golang-migrate (lines 376-382)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Dependency Injection (lines 275-278)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Logging (lines 526-533)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Rate Limiter (lines 407-421)]
- [Source: _bmad-output/planning-artifacts/epics.md — Story 2.1 AC (lines 365-381)]
- [Source: _bmad-output/planning-artifacts/epics.md — Additional Requirements (lines 111-136)]

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
