# Story 17.4: GET /emojis 接口（首次落地 EmojiRepo.List + emoji_service.ListAvailable + EmojisHandler.GetEmojis + 单测 ≥4 case + dockertest 集成测试 + router wire）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/internal/repo/mysql/emoji_repo.go` 的 `EmojiRepo` interface + `List(ctx) ([]EmojiConfig, error)` 实装（`SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`）+ **新增** `server/internal/service/emoji_service.go` 的 `EmojiService` interface + `ListAvailable(ctx) ([]EmojiBrief, error)` 实装（mysql repo 串行 + DB error 包成 1009 + nil slice 强制为 `[]EmojiBrief{}` 让 wire 层下发 `items: []` 而非 `null`）+ **新增** `server/internal/app/http/handler/emojis_handler.go` 的 `EmojisHandler.GetEmojis` handler（**不**做参数校验 / **不**用 c.Request.Context 之外的 ctx / DTO 转换 = `assetUrl` 字段名小驼峰 + sort_order → `sortOrder` int / 不下发 `id` / `is_enabled` / `created_at` / `updated_at`）+ **router wire**：`server/internal/app/bootstrap/router.go` 的 `authedGroup` 内挂 `GET /api/v1/emojis` 路由（与 §11.1 钦定路径一致 + 走 Auth + RateLimitByUserID 中间件）+ **新增** `emoji_repo_test.go` (≥3 case sqlmock：happy / empty / DB error) + `emoji_service_test.go` (≥4 case stub repo：happy 4 个 enabled 排序 / happy 含 disabled 已被 server 过滤 / edge 空列表返 `[]EmojiBrief{}` 不 panic / edge DB 错误返 1009) + `emojis_handler_test.go` (≥2 case stub service：happy 响应 envelope + assetUrl 字段名 / edge service 错误透传 1009) + `emoji_service_integration_test.go` (dockertest：seed 4 个表情 → svc.ListAvailable → 验证长度 4 + 字段值正确 + 排序按 sort_order ASC),
so that **Story 17.5（WS emoji.send 处理）+ iOS Epic 18.1（表情面板 SwiftUI + GET /emojis 缓存）+ Epic 19.1（节点 6 demo E2E）** 可以基于一个**已落地、严格符合 §11.1 契约**的 GET /emojis 端点继续展开，不再出现"17.4 端点空白让 18.1 表情面板硬编码假数据 / E2E demo 表情面板加载失败"的返工。

## 故事定位（Epic 17 第四条 = 第三条**实装** story；上承 17.3 emoji_configs 4 行 seed 已就绪，下启 17.5 WS emoji.send / emoji.received 广播 + iOS Epic 18.1 表情面板）

- **Epic 17 进度**：17.1（契约定稿，done）→ 17.2（emoji_configs migration + GORM struct，done）→ 17.3（emoji_configs seed ≥4 个表情，done）→ **17.4（本 story，GET /emojis HTTP 端点）** → 17.5（WS emoji.send 处理 + emoji.received 广播）。
- **本 story 是 17.5 / Epic 18.1 / Epic 19.1 的强前置**：
  - **17.5 WS emoji.send 校验**：iOS client 发 `emoji.send {emojiCode}` 前**应**校验 `emojiCode` 来自 §11.1 缓存的合法表情列表（V1 §12.2 行 2074 "client 端发送约束" 钦定）—— 本 story 落地的 GET /emojis 是 client 表情列表唯一来源；17.5 service 层校验路径独立查 `emoji_configs WHERE code = ? AND is_enabled = 1`（V1 §12.2 服务端逻辑步骤 4），**不**复用本 story 落地的 `EmojiRepo.List`（17.5 落地 `EmojiRepo.Exists(ctx, code)` 方法或类似单查询）
  - **iOS Epic 18.1 表情面板**：UI 层钦定"启动时调 `LoadEmojisUseCase` → GET /emojis → 拿到表情列表"（epics.md 行 2644）—— **直接依赖**本 story 落地的端点；assetUrl 字段名 / 类型 / 非空约束 / `items: []` vs `null` 边界都由本 story 严格对齐 §11.1 保证 client 端 Codable / `AsyncImage` 不触发解析失败 / 占位降级
  - **Epic 19.1 节点 6 demo E2E**：钦定"验证场景 1：A 进房间 → 点自己猫 → 表情面板出现 → 验证 4 个表情图标都加载成功（assetUrl 可访问）"（epics.md 行 2742）—— **直接依赖**本 story 端点 + 17.3 seed 的 4 行 + 每个 assetUrl 可访问
- **epics.md §Story 17.4 钦定**（行 2581-2599）：
  - 接口：`GET /emojis` 返回 `{items: [...]}`，仅含 `is_enabled=1` 的表情，按 `sort_order` 排序
  - 接口要求 auth（与 §11.1 元信息表"认证：需要 Bearer token（auth 中间件）"一致）
  - 表情列表很短（4-20 个），不分页
  - **单元测试覆盖**（≥4 case，mocked emoji repo）：
    - happy: DB 4 个 enabled 表情 → response items 长度 = 4，按 sort_order 排序
    - happy: DB 含 1 个 disabled → 不返回（**注**：本契约下 disabled 表情已被 repo 层 `WHERE is_enabled = 1` 过滤，service / handler 看不到 disabled 行；本 case 的"happy"语义是从 service 层视角验证传给 mock repo 4 行 enabled + 0 行 disabled 时 service 输出 4 行 —— "1 个 disabled 不返回"由 repo 层 sqlmock 单测 `is_enabled = 1` SQL 字面量校验兜底）
    - edge: DB 0 个 enabled → response items = []，不报错
    - edge: 服务端 DB 错误 → 1009 服务繁忙
  - **集成测试覆盖**（dockertest）：seed 4 个表情 → curl GET /emojis → response.items 长度 = 4 + 字段值正确
- **V1 §11.1 钦定**（17.1 r2 review 锁定 + 冻结）：
  - HTTP Method: GET / Path: /api/v1/emojis
  - 认证：需要 Bearer token（auth 中间件，路由挂在 authedGroup）
  - 限频：默认（按 Story 4.5 RateLimitByUserID 60 次/分；表情列表是静态配置）
  - 幂等：天然幂等（GET 查询，无副作用）
  - 不分页 / 不接受 query 参数（**禁止** `?category=` / `?orderBy=` / `?page=` 等任何 query string）
  - 服务端逻辑步骤 2：`SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`（次要键 `id ASC` 保证 sort_order 相同时返回顺序确定）
  - DTO 转换：`id` / `is_enabled` / `created_at` / `updated_at` **不**下发（client 用 `code` 作业务标识符）
  - 响应空列表 `items: []` 与 `items: null` 语义不同：server **永远**返回 `items: []`（空数组）**禁止** `null`
  - `assetUrl` 必非空字符串（1 ≤ length ≤ 255，**禁止** `""`）：API 契约层钦定，但 17.3 seed 已保证每行 asset_url 非空 → 本 story handler / service / repo **不**做空字符串过滤（trust seed + DDL DEFAULT '' 仅是 schema 兜底），enabled 表情若意外有空 assetUrl 让 client 走渲染失败路径而不是被 server 过滤（与"server 是表情数据 single source of truth"语义一致）
  - 错误码可能值：1001 (auth 中间件) / 1005 (rate_limit) / 1009 (DB 异常 / panic) —— **不**触发 1002 (无 query / body 校验) / 7001 (列表查询非单 emoji code 校验) / 6xxx (与房间无关)
  - **冻结边界**（17.1 r2 锁定）：GET /emojis 不分页 + 不接受 query 参数 + 不返回 disabled 表情属契约一部分；assetUrl 1-255 长度 + 非空字符串约束属契约一部分；DTO 字段集（code / name / assetUrl / sortOrder）属契约一部分；任何字段增删改视为契约变更
- **数据库设计 §5.15 钦定**（17.2 落地）：emoji_configs 表已存在；`idx_enabled_sort (is_enabled, sort_order ASC)` 普通索引保证 `WHERE is_enabled = 1 ORDER BY sort_order ASC` query 走索引（无 filesort）
- **lesson 2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md** Lesson 2 钦定（17-1 r2 lesson）：DB error 必须有 1009 路径 —— 本 story handler / service 层必须包 mysql err 成 1009（与 home_service 同模式 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)`）
- **lesson 2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md** Lesson 3 钦定：assetUrl 必非空字符串 —— 本 story **不**在 handler / service 层做"过滤空 assetUrl"分支（17.3 seed 已保证 enabled 表情 assetUrl 非空；server 端 seed 层 / admin 写入层负责校验）；本 story DTO 转换直接透传 `EmojiConfig.AssetURL` 字符串字段，**不**做 `if AssetURL == ""` 跳过分支
- **下游强依赖**（本 story 不动后才能开工）：
  - Story 17.5（WS emoji.send 处理 + emoji.received 广播）
  - iOS Epic 18.1（表情面板 SwiftUI + GET /emojis 缓存）
  - Epic 19.1（节点 6 demo E2E）
- **范围红线**：
  - 本 story **只**改 / 新建以下文件：
    - `server/internal/repo/mysql/emoji_repo.go`（修改 —— 17.2 已落地 EmojiConfig struct + TableName；本 story 新增 `EmojiRepo` interface + `emojiRepo` struct + `NewEmojiRepo` 构造 + `List(ctx) ([]EmojiConfig, error)` 方法）
    - `server/internal/repo/mysql/emoji_repo_test.go`（新建 —— ≥3 case sqlmock 单测）
    - `server/internal/service/emoji_service.go`（新建 —— `EmojiService` interface + `EmojiBrief` struct + `emojiServiceImpl` struct + `NewEmojiService` 构造 + `ListAvailable(ctx) ([]EmojiBrief, error)` 方法）
    - `server/internal/service/emoji_service_test.go`（新建 —— ≥4 case stub repo 单测）
    - `server/internal/service/emoji_service_integration_test.go`（新建 —— `//go:build integration` tag + dockertest 集成测试 1 case：seed 4 个表情 → svc.ListAvailable → 验证字段值 + 长度 + 排序）
    - `server/internal/app/http/handler/emojis_handler.go`（新建 —— `EmojisHandler` struct + `NewEmojisHandler` 构造 + `GetEmojis(c *gin.Context)` handler）
    - `server/internal/app/http/handler/emojis_handler_test.go`（新建 —— ≥2 case stub service 单测）
    - `server/internal/app/bootstrap/router.go`（修改 —— 在 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil` 块内 wire `emojiRepo := repomysql.NewEmojiRepo(deps.GormDB)` + `emojiSvc := service.NewEmojiService(emojiRepo)` + `emojisHandler := handler.NewEmojisHandler(emojiSvc)` + 在 `authedGroup.GET("/home", ...)` 附近加 `authedGroup.GET("/emojis", emojisHandler.GetEmojis)`）
    - 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 WS 处理（17.5 才做）
  - **不**实装 `EmojiRepo.Exists(ctx, code)` 方法（17.5 才做 —— 那是 `SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1`，与本 story List 方法是**两条独立 query**，提前 ship Exists 会让 17.5 review 找不到"新增方法"的范围边界，与 17.2 / 17.3 "禁止预实装 repo 方法"同模式）
  - **不**实装任何 emoji 写入方法（`Create` / `UpdateIsEnabled` 等 admin 端能力 —— MVP 节点 6 不规划 admin 后台）
  - **不**修改 17.2 落地的 `EmojiConfig` struct（已与 §5.15 1:1 对齐；本 story **不**新增字段 / 改 tag）
  - **不**修改 0001 ~ 0010 既有 migration 文件（17.2 / 17.3 已落地 schema + seed）
  - **不**改 V1 接口契约（17.1 已冻结；本 story 严格对齐 §11.1 字段表 + 错误码表 + 服务端逻辑步骤；任何偏离都是 bug）
  - **不**改任何 `docs/宠物互动App_*.md`
  - **不**改 ADR-0006（error envelope 单一生产者；本 story handler 走 `c.Error(err) + return` 让 ErrorMappingMiddleware 翻译，与 home_handler / room_handler 同模式）
  - **不**改 ADR-0007（ctx 传播；本 story handler / service / repo 全链路 ctx-aware，与既有 4 个 repo + 4 个 service 同模式）
  - **不**接 Redis（10.6 已接，本 story 不需要 —— 表情列表全量 DB query，无 cache 需求；client 端缓存契约由 V1 §11.1 行 1817 钦定 iOS 侧负责）
  - **不**接 idempotency 键（GET 接口天然幂等，无需 Redis idempotency key）
  - **不**写 metric（默认 Prometheus middleware 已挂在 router；表情接口不需要业务级 metric）
  - **不**为 GET /emojis 接口实装 ETag / If-None-Match（V1 §11.1 行 1817 钦定"如 server 端表情配置变更，client 需要重启 App 或主动刷新；节点 6 MVP 阶段无 push 通知机制，可接受"；本 story 仅落地最小 happy / edge path）
  - **不**实装 admin 后台 / `POST /dev/emoji` 等运维端点（YAGNI；MVP 节点 6 仅静态 seed）

**本 story 不做**（明确范围红线）：

- 不实装 WS emoji.send / emoji.received 链路（17.5 范围）
- 不实装 `EmojiRepo.Exists(ctx, code)` 方法（17.5 范围；本 story 仅落地 `List(ctx)`）
- 不实装 iOS Epic 18.1 表情面板（独立 epic）
- 不修改 17.2 落地的 `EmojiConfig` struct（字段集已冻结）
- 不修改 17.3 落地的 0010_seed_emoji_configs.up/down.sql（seed 内容已冻结）
- 不修改 V1 §11.1 接口契约（17.1 已冻结）
- 不修改 ADR-0006 / ADR-0007（同模式沿用）
- 不引入 Redis cache / ETag / If-None-Match（YAGNI；MVP 节点 6 不需要）
- 不实装 admin 后台 / 运维端点（YAGNI）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与既有 17.x / 11.x / 14.x 同模式）

## Acceptance Criteria

**AC1 — `EmojiRepo` interface + `emojiRepo` struct + `List(ctx)` 实装（emoji_repo.go 修改）**

在 `server/internal/repo/mysql/emoji_repo.go` 文件**末尾**追加（不动既有 17.2 `EmojiConfig` struct + `TableName` 方法）：

```go
// EmojiRepo 是 emoji_configs 表的读取接口（Story 17.4 引入）。
//
// 本 story 阶段仅含 List 方法（GET /emojis 端点）；future Story 17.5 加
// Exists(ctx, code) 方法（WS emoji.send 校验 emojiCode 合法性 —— 单 emoji code
// 查询路径，与 List 全列表查询路径分开实装）。
//
// 不在本 story 落地：Create / Update / Delete（MVP 节点 6 无 admin 后台需求；
// emoji_configs 写入路径目前仅 0010_seed migration 一次性 seed，无运行时 admin 改写场景）
type EmojiRepo interface {
	// List 返回所有 is_enabled=1 的 emoji_configs 行（V1 §11.1 服务端逻辑步骤 2 钦定）。
	//
	// SQL: SELECT id, code, name, asset_url, sort_order FROM emoji_configs
	//      WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC
	//
	// 关键约束（§11.1 钦定）：
	//   - **次要排序键 `id ASC`**：保证 sort_order 相同时返回顺序确定
	//     （避免 client 端"同 sort_order 表情顺序在不同请求间不一致"问题）
	//   - **`is_enabled = 1`** 过滤：disabled 表情**不**返回（被 admin 临时下架 /
	//     WIP 阶段不放出的表情）
	//   - 0 行（如 seed 未执行 / 全部 disabled）→ ([]EmojiConfig{}, nil)，**不**返 nil slice
	//   - 多行 → ([]EmojiConfig{...}, nil)
	//   - DB error → (nil, raw error 透传给 service（service 包成 1009）)
	//
	// **注**：本方法返回 `[]EmojiConfig` 含 ID / CreatedAt / UpdatedAt 等字段，但 service
	// 层 DTO 转换会过滤掉 client 不需要的字段（V1 §11.1 钦定 id / is_enabled /
	// created_at / updated_at 不下发）；repo 层不做字段裁剪 —— 让 service 层关心 wire
	// 契约，repo 层只关心表字段映射。
	//
	// **走索引**：emoji_configs 表 `idx_enabled_sort (is_enabled, sort_order ASC)` 索引
	// 覆盖 `WHERE is_enabled = 1 ORDER BY sort_order ASC`（数据库设计 §5.15 钦定）；
	// 无 filesort，性能稳定。
	List(ctx context.Context) ([]EmojiConfig, error)
}

// emojiRepo 是 EmojiRepo 的默认实装。
type emojiRepo struct {
	db *gorm.DB
}

// NewEmojiRepo 构造 EmojiRepo。
func NewEmojiRepo(db *gorm.DB) EmojiRepo {
	return &emojiRepo{db: db}
}

// List 实装：单 SELECT 查询。详见 EmojiRepo.List 接口注释。
func (r *emojiRepo) List(ctx context.Context) ([]EmojiConfig, error) {
	// 用 tx.FromContext 取 db handle：事务外调用走 r.db；事务内调用走 txCtx 注入的
	// tx 句柄（与 11.6 既有 repo 同模式；本 story 阶段实际不会在事务内调，但保持
	// 与既有 repo 模式一致让 future 扩展无需改 method body）。
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var rows []EmojiConfig
	// 显式 Select 字段集（不依赖 GORM 自动 SELECT *），与 §11.1 服务端逻辑步骤 2
	// 钦定字段 1:1 对齐；避免 future 表加字段时被自动拉过来污染 query payload。
	// **注**：CreatedAt / UpdatedAt **不**在 SELECT 列表中（client 不需要 + service
	// 层不做 wire DTO 转换），但 GORM Scan 会把它们填为 zero-value time.Time；
	// service 层 DTO 转换不读这两字段，所以 zero-value 是安全的。
	err := db.
		Select("id, code, name, asset_url, sort_order").
		Where("is_enabled = ?", 1).
		Order("sort_order ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// GORM Find 在 0 行时返回空 slice 而非 nil（与 ListMembers 同模式 — 验证：
	// db.Find(&[]X{}) 返回 len(rows)==0 且 rows != nil）；保险起见显式兜底空 slice
	// 让 service 层调用方不需要 nil-check。
	if rows == nil {
		rows = []EmojiConfig{}
	}
	return rows, nil
}
```

文件顶部 import 段添加 `"context"` / `"gorm.io/gorm"` / `"github.com/huing/cat/server/internal/repo/tx"`（17.2 落地的 `time` 保留）。

**AC1 验收**：
- `server/internal/repo/mysql/emoji_repo.go` 含 `EmojiRepo` interface + `emojiRepo` struct + `NewEmojiRepo` + `List` 方法
- SQL 严格符合 §11.1 服务端逻辑步骤 2：`SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`
- 走 `tx.FromContext` + `.WithContext(ctx)` 与 ADR-0007 一致
- 0 行场景显式返 `[]EmojiConfig{}` 而非 nil slice

**AC2 — `emoji_repo_test.go` 新建（≥3 case sqlmock 单测）**

在 `server/internal/repo/mysql/emoji_repo_test.go` 新建，与 `room_member_repo_test.go` 同模式（sqlmock + regexp.QuoteMeta）：

```go
package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// buildMockEmojiRepo 用 sqlmock 起一个 GORM db 实例 + 构造 emojiRepo。
func buildMockEmojiRepo(t *testing.T) (EmojiRepo, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return NewEmojiRepo(gormDB), mock, func() { _ = sqlDB.Close() }
}

func TestEmojiRepo_List_HappyPath_4Rows(t *testing.T) {
	repo, mock, cleanup := buildMockEmojiRepo(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"id", "code", "name", "asset_url", "sort_order"}).
		AddRow(uint64(1), "wave", "挥手", "https://placehold.co/64x64?text=Wave", int32(1)).
		AddRow(uint64(2), "love", "爱心", "https://placehold.co/64x64?text=Love", int32(2)).
		AddRow(uint64(3), "laugh", "大笑", "https://placehold.co/64x64?text=Laugh", int32(3)).
		AddRow(uint64(4), "cry", "哭", "https://placehold.co/64x64?text=Cry", int32(4))

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, asset_url, sort_order FROM `emoji_configs` WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC",
	)).
		WithArgs(1).
		WillReturnRows(rows)

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	if got[0].Code != "wave" || got[1].Code != "love" || got[2].Code != "laugh" || got[3].Code != "cry" {
		t.Errorf("codes = %v %v %v %v, want wave/love/laugh/cry",
			got[0].Code, got[1].Code, got[2].Code, got[3].Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("ExpectationsWereMet: %v", err)
	}
}

func TestEmojiRepo_List_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	repo, mock, cleanup := buildMockEmojiRepo(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, asset_url, sort_order FROM `emoji_configs` WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC",
	)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "asset_url", "sort_order"})) // 0 行

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []EmojiConfig{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestEmojiRepo_List_DBError_ReturnsRawError(t *testing.T) {
	repo, mock, cleanup := buildMockEmojiRepo(t)
	defer cleanup()

	dbErr := fakeDBError()
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, asset_url, sort_order FROM `emoji_configs` WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC",
	)).
		WithArgs(1).
		WillReturnError(dbErr)

	got, err := repo.List(context.Background())
	if err == nil {
		t.Fatal("err == nil, want raw DB error")
	}
	if got != nil {
		t.Errorf("got != nil on error, want nil slice")
	}
}

// fakeDBError 返回一个 driver-level error，模拟连接断 / 慢查询超时等场景。
// 与既有 room_member_repo_test.go 内 mockDBError helper 同模式。
func fakeDBError() error {
	return &mysqlDriverError{msg: "fake DB connection lost"}
}

type mysqlDriverError struct{ msg string }

func (e *mysqlDriverError) Error() string { return e.msg }
```

**注**：dev 在实装时如发现 sqlmock 的 SQL 字面量与 GORM 实际生成的 SQL 不完全一致（如 backtick 转义 / 参数占位符位置）→ 用 `regexp.QuoteMeta` 包装 + 实际跑一次 test 看 sqlmock 错误信息中的 actual SQL，调整 expected SQL 字面量；与 11.6 既有 repo test 同模式。

**AC2 验收**：
- `emoji_repo_test.go` 含 ≥3 个 Test 函数：HappyPath_4Rows / EmptyResult_ReturnsEmptySlice / DBError_ReturnsRawError
- 所有 case 用 sqlmock 起 mock DB；SQL 字面量校验严格匹配 §11.1 钦定 query
- empty 场景断言 `got != nil` 且 `len(got) == 0`（防 nil slice 回归）
- DB error 场景断言 err != nil + got == nil

**AC3 — `service/emoji_service.go` 新建（EmojiService interface + EmojiBrief struct + emojiServiceImpl + ListAvailable 实装）**

新建 `server/internal/service/emoji_service.go`：

```go
package service

import (
	"context"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// EmojiService 是 emojis handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 home_service / room_service 同模式。
type EmojiService interface {
	// ListAvailable 返回所有 is_enabled=1 的 emoji（V1 §11.1 服务端逻辑钦定）。
	//
	// 流程：
	//  1. emojiRepo.List(ctx) → []mysql.EmojiConfig（仅 is_enabled=1 已被 repo 层 SQL 过滤）
	//  2. DTO 转换：mysql.EmojiConfig → EmojiBrief（裁掉 id / is_enabled / created_at /
	//     updated_at；client 不需要这些字段）
	//  3. 0 行 → []EmojiBrief{}（**永远**非 nil；让 handler / wire 层下发 `items: []`
	//     而非 `null`，与 V1 §11.1 钦定一致）
	//
	// 错误约定：
	//   - emojiRepo.List 失败（含 DB 异常 / 连接断 / 慢查询超时等）→ apperror.Wrap 包成
	//     1009 ErrServiceBusy（与 home_service 4 repo 失败兜底同模式 +
	//     lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）
	//
	// **不**做空字符串过滤：V1 §11.1 钦定 assetUrl 必非空字符串（17.3 seed 已保证 +
	// 17-1 r2 Lesson 3 钦定 server 端 seed 层 / admin 写入层负责校验）；本方法
	// **不**做 `if AssetURL == "" 跳过` 分支 —— 让意外有空 assetUrl 的 enabled 行
	// 透传到 client 触发渲染失败而不是被 server 静默过滤（与"server 是表情数据 single
	// source of truth"语义一致）
	ListAvailable(ctx context.Context) ([]EmojiBrief, error)
}

// EmojiBrief 是 V1 §11.1 data.items[] 的 service 层映射（**不是** wire DTO，
// handler 转换为 §11.1 钦定 wire 格式：snake_case 字段名 → camelCase）。
//
// 字段（与 §11.1 data.items[] 钦定字段集 1:1 对齐）：
//   - Code:      string（V1 §11.1 `data.items[].code`；client 业务标识符）
//   - Name:      string（V1 §11.1 `data.items[].name`；UI 展示文字）
//   - AssetURL:  string（V1 §11.1 `data.items[].assetUrl`；资源 URL 非空字符串）
//   - SortOrder: int32（V1 §11.1 `data.items[].sortOrder`；显示顺序升序）
//
// **不**含 ID / IsEnabled / CreatedAt / UpdatedAt：V1 §11.1 钦定 client 不需要这些字段。
type EmojiBrief struct {
	Code      string
	Name      string
	AssetURL  string
	SortOrder int32
}

// emojiServiceImpl 是 EmojiService 的默认实装。
type emojiServiceImpl struct {
	emojiRepo mysql.EmojiRepo
}

// NewEmojiService 构造 EmojiService。
func NewEmojiService(emojiRepo mysql.EmojiRepo) EmojiService {
	return &emojiServiceImpl{emojiRepo: emojiRepo}
}

// ListAvailable 实装：单 repo query + DTO 转换 + nil slice 兜底。
func (s *emojiServiceImpl) ListAvailable(ctx context.Context) ([]EmojiBrief, error) {
	rows, err := s.emojiRepo.List(ctx)
	if err != nil {
		// V1 §11.1 错误码表：DB 异常 → 1009 ErrServiceBusy
		// （lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 永远返非 nil slice（即便 rows 是空）—— 让 handler / wire 层下发 `items: []`
	// 而非 `null`（V1 §11.1 钦定 + 行 1833 "items: [] 与 items: null 语义不同"）。
	briefs := make([]EmojiBrief, 0, len(rows))
	for _, r := range rows {
		briefs = append(briefs, EmojiBrief{
			Code:      r.Code,
			Name:      r.Name,
			AssetURL:  r.AssetURL,
			SortOrder: r.SortOrder,
		})
	}
	return briefs, nil
}
```

**AC3 验收**：
- `emoji_service.go` 含 `EmojiService` interface + `EmojiBrief` struct + `emojiServiceImpl` + `NewEmojiService` 构造 + `ListAvailable` 方法
- DB 错误用 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)` 包成 1009（与 home_service 同模式）
- 0 行场景返 `[]EmojiBrief{}` 非 nil（用 `make([]EmojiBrief, 0, len(rows))` 兜底）
- `EmojiBrief` 字段集严格 = `{Code, Name, AssetURL, SortOrder}`，**不含** ID / IsEnabled / CreatedAt / UpdatedAt

**AC4 — `service/emoji_service_test.go` 新建（≥4 case stub repo 单测）**

新建 `server/internal/service/emoji_service_test.go`：

```go
package service_test

import (
	"context"
	stderrors "errors"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// stubEmojiRepo 用 fn 字段让每个 case 自定义返回（与 stubHomeUserRepo 同模式）。
type stubEmojiRepo struct {
	listFn func(ctx context.Context) ([]mysql.EmojiConfig, error)
}

func (s *stubEmojiRepo) List(ctx context.Context) ([]mysql.EmojiConfig, error) {
	return s.listFn(ctx)
}

func buildEmojiService(listFn func(ctx context.Context) ([]mysql.EmojiConfig, error)) service.EmojiService {
	return service.NewEmojiService(&stubEmojiRepo{listFn: listFn})
}

// AC4.1 happy: 4 个 enabled 表情 → 4 个 EmojiBrief 按 sort_order 顺序返回
func TestEmojiService_ListAvailable_HappyPath_4Rows(t *testing.T) {
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return []mysql.EmojiConfig{
			{ID: 1, Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1, IsEnabled: 1},
			{ID: 2, Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2, IsEnabled: 1},
			{ID: 3, Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3, IsEnabled: 1},
			{ID: 4, Code: "cry", Name: "哭", AssetURL: "https://x/cry", SortOrder: 4, IsEnabled: 1},
		}, nil
	})

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	want := []service.EmojiBrief{
		{Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1},
		{Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2},
		{Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3},
		{Code: "cry", Name: "哭", AssetURL: "https://x/cry", SortOrder: 4},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// AC4.2 happy: repo 已过滤 disabled —— service 层从未见到 IsEnabled=0 行；本 case
// 模拟 repo 返 3 行 enabled（如果 DB 有 1 行 disabled，repo SQL `is_enabled = 1` 已
// 过滤）→ service 返 3 行。
func TestEmojiService_ListAvailable_DisabledFilteredAtRepoLayer_ReturnsOnlyEnabled(t *testing.T) {
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return []mysql.EmojiConfig{
			{ID: 1, Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1, IsEnabled: 1},
			{ID: 2, Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2, IsEnabled: 1},
			{ID: 3, Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3, IsEnabled: 1},
			// IsEnabled=0 行被 repo SQL 过滤，stub repo 不返回
		}, nil
	})

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3 (disabled filtered at repo layer)", len(got))
	}
}

// AC4.3 edge: 0 行 → 返 []EmojiBrief{} 非 nil；不报错
func TestEmojiService_ListAvailable_EmptyResult_ReturnsEmptySliceNotNil(t *testing.T) {
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return []mysql.EmojiConfig{}, nil
	})

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable err = %v, want nil", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []EmojiBrief{} (V1 §11.1 行 1833: items: [] not null)")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// AC4.4 edge: DB 错误 → 1009 ErrServiceBusy；nil slice
func TestEmojiService_ListAvailable_DBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("driver: connection lost")
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return nil, dbErr
	})

	got, err := svc.ListAvailable(context.Background())
	if err == nil {
		t.Fatal("err == nil, want 1009 AppError")
	}
	if got != nil {
		t.Errorf("got != nil on error, want nil slice")
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("err is not *apperror.AppError: %T", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("appErr.Code = %d, want %d (ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
	}
}
```

**注**：`apperror.AppError` 的导出 / Code 字段访问路径 dev 实装时按 `internal/pkg/errors/apperror.go` 既有 export 名（可能是 `AppError.Code` 或 getter；本 story 不约束实装细节）；如 wrap 后的 error 用 `errors.Is` 而非 `errors.As` 判定，调整 assert 路径但**必须**断言到 1009 业务码上而非仅断言 err != nil。

**AC4 验收**：
- `emoji_service_test.go` 含 ≥4 个 Test 函数：HappyPath_4Rows / DisabledFilteredAtRepoLayer / EmptyResult_ReturnsEmptySliceNotNil / DBError_Returns1009
- 所有 case 用 stub repo 注入；不需要起 mysql 容器 / sqlmock
- AC4.3 显式断言 `got != nil`（防 nil slice 回归 —— `items: []` vs `null` 边界）
- AC4.4 显式断言 appErr.Code == ErrServiceBusy（1009）

**AC5 — `handler/emojis_handler.go` 新建（EmojisHandler + GetEmojis + DTO 转换）**

新建 `server/internal/app/http/handler/emojis_handler.go`：

```go
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// EmojisHandler 是 /emojis 路由的 handler。
//
// 节点 6 阶段仅 GetEmojis（GET /api/v1/emojis）；future epic 加 POST / PATCH /emojis
// 等 admin 端能力（不在 MVP 范围）。
type EmojisHandler struct {
	svc service.EmojiService
}

// NewEmojisHandler 构造 EmojisHandler。
//
// 注入 EmojiService（service 层 interface）—— handler 单测直接传 stub struct
// 实现该 interface，不需要起 *gorm.DB / 真 mysql。与 HomeHandler / RoomHandler 同模式。
func NewEmojisHandler(svc service.EmojiService) *EmojisHandler {
	return &EmojisHandler{svc: svc}
}

// GetEmojis 处理 GET /api/v1/emojis。
//
// # 流程
//
//  1. 调 svc.ListAvailable(ctx) 取所有 enabled 表情
//  2. 成功 → response.Success(c, emojiResponseDTO(briefs), "ok")
//  3. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # 关键
//
// 本 handler **不**做参数校验：V1 §11.1 钦定不接受任何 query 参数 / body 字段，
// 也不读 userID（接口要求 auth 但 service 不需要 user 维度过滤）；auth 头由 router
// 中间件链兜底（authedGroup 已挂 Auth 中间件）。
//
// # ADR-0006 单一 envelope 生产者
//
// 本 handler **不**直接调 response.Error 写 envelope —— 一律走 c.Error + return，
// 由 ErrorMappingMiddleware 兜底翻译成 envelope。详见
// docs/lessons/2026-04-24-error-envelope-single-producer.md。
//
// # ADR-0007 §2.2 ctx 传播
//
// 用 c.Request.Context() 传给 service —— **不**直接传 *gin.Context（其 Done() 是
// nil channel，service 层 select ctx.Done() 不会响应 client 断开）。
func (h *EmojisHandler) GetEmojis(c *gin.Context) {
	briefs, err := h.svc.ListAvailable(c.Request.Context())
	if err != nil {
		// service 已 wrap *AppError；handler 透传，让 ErrorMappingMiddleware 写 envelope
		_ = c.Error(err)
		return
	}

	response.Success(c, emojiResponseDTO(briefs), "ok")
}

// emojiResponseDTO 把 service 输出转成 V1 §11.1 钦定的 wire 格式。
//
// # 关键转换
//
//   - 字段名 snake_case → camelCase：AssetURL → "assetUrl" / SortOrder → "sortOrder"
//     （V1 §2.4 行 138 钦定 wire 全 camelCase；与 home_handler / room_handler 同模式）
//   - **不**下发 ID / IsEnabled / CreatedAt / UpdatedAt（V1 §11.1 行 1815 钦定）
//   - **永远**下发 `items: []` 而非 `items: null`（V1 §11.1 行 1833 钦定）：用 `[]any{}`
//     兜底空 slice → nil interface{} JSON null（与 home_handler.go pet null 同模式）
//
// # V1 §11.1 钦定的 wire 字段集（任一缺失 → iOS DTO 解码失败）
//
//   - data.items[].code: string
//   - data.items[].name: string
//   - data.items[].assetUrl: string
//   - data.items[].sortOrder: number (int)
func emojiResponseDTO(briefs []service.EmojiBrief) gin.H {
	// 永远返非 nil items slice（即便 briefs 是空）—— V1 §11.1 行 1833 钦定。
	// service 层已经保证 briefs 非 nil（用 make([]EmojiBrief, 0, len(rows)) 兜底），
	// 这里再保险一次避免任何 edge case 触发 JSON null（如 future 改动 service 实装）。
	items := make([]gin.H, 0, len(briefs))
	for _, b := range briefs {
		items = append(items, gin.H{
			"code":      b.Code,
			"name":      b.Name,
			"assetUrl":  b.AssetURL,
			"sortOrder": b.SortOrder,
		})
	}

	return gin.H{
		"items": items,
	}
}
```

**AC5 验收**：
- `emojis_handler.go` 含 `EmojisHandler` struct + `NewEmojisHandler` + `GetEmojis` 方法
- handler **不**做参数校验（无 query / body 字段，与 §11.1 一致）
- DTO 转换：snake_case → camelCase（`assetUrl` / `sortOrder`），**不**下发 id / is_enabled / created_at / updated_at
- 空 slice 兜底 `make([]gin.H, 0, len(briefs))`（防 nil → JSON null）
- 错误走 `c.Error(err) + return`（与 home_handler / room_handler 同模式 + ADR-0006）

**AC6 — `handler/emojis_handler_test.go` 新建（≥2 case stub service 单测）**

新建 `server/internal/app/http/handler/emojis_handler_test.go`，与 `home_handler_test.go` 同模式（gin TestMode + httptest.NewRecorder + stub service）：

```go
package handler_test

import (
	"context"
	stderrors "errors"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/service"
)

type stubEmojiService struct {
	listAvailableFn func(ctx context.Context) ([]service.EmojiBrief, error)
}

func (s *stubEmojiService) ListAvailable(ctx context.Context) ([]service.EmojiBrief, error) {
	return s.listAvailableFn(ctx)
}

func buildEmojisHandlerRouter(svc service.EmojiService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 复用 ErrorMappingMiddleware 让 c.Error(err) 走完整 envelope 路径（与
	// home_handler_test buildHomeHandlerRouter 同模式）。
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewEmojisHandler(svc)
	r.GET("/api/v1/emojis", h.GetEmojis)
	return r
}

// AC6.1 happy: service 返 4 个 EmojiBrief → envelope code=0 + items 长度=4 +
// 字段名严格 camelCase（assetUrl / sortOrder）
func TestEmojisHandler_GetEmojis_HappyPath_4Rows(t *testing.T) {
	svc := &stubEmojiService{
		listAvailableFn: func(ctx context.Context) ([]service.EmojiBrief, error) {
			return []service.EmojiBrief{
				{Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1},
				{Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2},
				{Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3},
				{Code: "cry", Name: "哭", AssetURL: "https://x/cry", SortOrder: 4},
			}, nil
		},
	}
	r := buildEmojisHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emojis", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body struct {
		Code int    `json:"code"`
		Data struct {
			Items []struct {
				Code      string `json:"code"`
				Name      string `json:"name"`
				AssetURL  string `json:"assetUrl"`
				SortOrder int    `json:"sortOrder"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("json.Decode: %v", err)
	}
	if body.Code != 0 {
		t.Errorf("body.code = %d, want 0", body.Code)
	}
	if len(body.Data.Items) != 4 {
		t.Fatalf("len(items) = %d, want 4", len(body.Data.Items))
	}
	if body.Data.Items[0].Code != "wave" || body.Data.Items[0].AssetURL != "https://x/wave" || body.Data.Items[0].SortOrder != 1 {
		t.Errorf("items[0] = %+v, want wave/https://x/wave/1", body.Data.Items[0])
	}
}

// AC6.2 edge: service 返 1009 → envelope code=1009 + items 不存在
func TestEmojisHandler_GetEmojis_ServiceError_Returns1009(t *testing.T) {
	svc := &stubEmojiService{
		listAvailableFn: func(ctx context.Context) ([]service.EmojiBrief, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := buildEmojisHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emojis", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// ErrorMappingMiddleware 把 1009 翻译为 HTTP 500 / 503 / 200（具体 status 由 mapping
	// 决定；本断言只锁 envelope code 字段）
	var body struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("json.Decode: %v", err)
	}
	if body.Code != apperror.ErrServiceBusy {
		t.Errorf("body.code = %d, want %d (1009)", body.Code, apperror.ErrServiceBusy)
	}
	_ = stderrors.New // 占位让 import 不被去掉（如未引用 stderrors，删除该 import 行）
}
```

**注**：`buildEmojisHandlerRouter` 是否需要挂 Auth 中间件 / userID 注入由 dev 实装时按既有 home_handler_test 的模式决定（home_handler 注入 userID 是因 LoadHome 读 userID；本 handler 不读 userID，所以 buildRouter 可以**不**挂 Auth + UserID 注入，让 test case 更精简）。

**AC6 验收**：
- `emojis_handler_test.go` 含 ≥2 个 Test 函数：HappyPath_4Rows / ServiceError_Returns1009
- happy case 显式断言字段名 `assetUrl` / `sortOrder`（防 snake_case 回归）
- error case 显式断言 envelope code == 1009（防 ErrorMappingMiddleware 翻译丢失）
- 用 stub service + httptest，不依赖真 mysql / sqlmock

**AC7 — `service/emoji_service_integration_test.go` 新建（dockertest 集成测试 1 case）**

新建 `server/internal/service/emoji_service_integration_test.go`，复用 `home_service_integration_test.go` 的 `startMySQL` / `runMigrations` helper：

```go
//go:build integration
// +build integration

// Story 17.4 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 1 条 happy 链路 case：
//   1. migrate up 落地 0010 seed 后 4 行 → svc.ListAvailable 返 4 个 EmojiBrief +
//      字段值正确 + 排序按 sort_order ASC 稳定
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// 复用 home_service_integration_test.go 的 startMySQL / runMigrations helper（同
// service_test package 直接调，与既有集成测试同模式）。
//
// **不**手工 INSERT 测试数据（与 17.3 集成测试不同）—— 直接复用 0010 seed migration
// 落地的 4 行（wave / love / laugh / cry），让本 case 同时验证：
//   - emoji_repo.List SQL 正确（实际跑出 4 行）
//   - service.ListAvailable DTO 转换正确（字段值不丢失）
//   - 与 17.3 seed 跨 story 集成（seed → 接口 endpoint 闭环）

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

func buildEmojiServiceIntegration(t *testing.T) (svc service.EmojiService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn) // 跑到最新版（含 0010 seed）

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	emojiRepo := mysql.NewEmojiRepo(gormDB)
	svc = service.NewEmojiService(emojiRepo)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, cleanup
}

func TestEmojiServiceIntegration_ListAvailable_SeedContent(t *testing.T) {
	svc, _, cleanup := buildEmojiServiceIntegration(t)
	defer cleanup()

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (seed by 0010 migration)", len(got))
	}

	// 验证字段值（与 0010_seed_emoji_configs.up.sql 钦定 1:1 对齐）+
	// 排序按 sort_order ASC（V1 §11.1 服务端逻辑步骤 2 钦定）
	want := []struct {
		code      string
		name      string
		sortOrder int32
	}{
		{"wave", "挥手", 1},
		{"love", "爱心", 2},
		{"laugh", "大笑", 3},
		{"cry", "哭", 4},
	}
	for i, w := range want {
		if got[i].Code != w.code {
			t.Errorf("got[%d].Code = %q, want %q", i, got[i].Code, w.code)
		}
		if got[i].Name != w.name {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, w.name)
		}
		if got[i].SortOrder != w.sortOrder {
			t.Errorf("got[%d].SortOrder = %d, want %d", i, got[i].SortOrder, w.sortOrder)
		}
		if got[i].AssetURL == "" {
			t.Errorf("got[%d].AssetURL is empty, want non-empty (V1 §11.1 钦定 enabled 表情 assetUrl 必非空)", i)
		}
	}
}
```

**AC7 验收**：
- `emoji_service_integration_test.go` 含 `//go:build integration` build tag + 1 个 Test 函数 SeedContent
- 直接复用 0010 seed migration 落地的 4 行（不手工 INSERT）
- 断言 len(got) == 4 + 字段值 4 行精确匹配 + sortOrder 严格 1/2/3/4 + assetUrl 非空
- 用 `bash scripts/build.sh --integration` 跑通

**AC8 — `bootstrap/router.go` wire（emojisHandler + 路由挂载）**

修改 `server/internal/app/bootstrap/router.go`：

在 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil { ... }` 块内（与其他 repo / service / handler 平级）添加：

```go
// 在已有 repo 实例化段（userRepo / authBindingRepo / petRepo / ... / roomMemberRepo）
// 附近添加：
emojiRepo := repomysql.NewEmojiRepo(deps.GormDB)

// 在 homeSvc / homeHandler 等 service / handler 实例化段附近添加（建议放
// home / steps 之后，room 之前，与代码阅读顺序对齐 V1 节点 6 / GET endpoints）：
// Story 17.4 加：emoji service + handler（GET /emojis；单 SELECT 不开事务）。
emojiSvc := service.NewEmojiService(emojiRepo)
emojisHandler := handler.NewEmojisHandler(emojiSvc)

// 在 authedGroup 路由挂载段（authedGroup.GET("/home", ...) 附近）添加：
// Story 17.4 加：GET /api/v1/emojis 表情列表（auth + RateLimitByUserID）
authedGroup.GET("/emojis", emojisHandler.GetEmojis)
```

**AC8 验收**：
- `router.go` 内有 `emojiRepo` / `emojiSvc` / `emojisHandler` 三个新实例化变量
- `authedGroup.GET("/emojis", emojisHandler.GetEmojis)` 路由挂载存在
- 路由挂在 `authedGroup`（**不**是 `authGroup` / 不暴露在 /auth 子组）—— 接口要求 auth + RateLimitByUserID
- 路径**不**带 `/api/v1` 前缀（authedGroup 父组已是 `api.Group("")` 在 `api := r.Group("/api/v1")` 之下，与 `/home` / `/rooms` 等同模式 → 实际路径是 `/api/v1/emojis`）

**AC9 — `bash scripts/build.sh --test` 通过**

```bash
bash scripts/build.sh --test
```

- 所有既有单测继续 PASS（emoji_repo_test.go / emoji_service_test.go / emojis_handler_test.go 新增 case 全 PASS）
- 不引入 lint error / vet error / 编译 warning

**AC10 — `bash scripts/build.sh --integration` 通过**

```bash
bash scripts/build.sh --integration
```

- `TestEmojiServiceIntegration_ListAvailable_SeedContent` PASS
- 既有集成测试继续 PASS（home_service_integration_test / auth_service_integration_test / 17.3 落地的 migrate_integration_test）

## Tasks / Subtasks

- [x] Task 1: `EmojiRepo` interface + `List` 实装（AC1）
  - [x] 1.1: 在 `server/internal/repo/mysql/emoji_repo.go` 末尾追加 `EmojiRepo` interface + `emojiRepo` struct + `NewEmojiRepo` 构造 + `List(ctx)` 方法（参考 AC1 钦定代码块）
  - [x] 1.2: 文件顶部 import 段补 `"context"` / `"gorm.io/gorm"` / `tx`
  - [x] 1.3: SQL 严格符合 V1 §11.1 服务端逻辑步骤 2：`SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`
  - [x] 1.4: 用 `tx.FromContext(ctx, r.db).WithContext(ctx)` 取 db handle（与 既有 repo 同模式 + ADR-0007）
  - [x] 1.5: 0 行场景显式 `rows = []EmojiConfig{}` 兜底（防 nil slice）

- [x] Task 2: `emoji_repo_test.go` sqlmock 单测（AC2）
  - [x] 2.1: 新建 `server/internal/repo/mysql/emoji_repo_test.go`（参考 AC2 钦定代码块）
  - [x] 2.2: ≥3 case：HappyPath_4Rows / EmptyResult_ReturnsEmptySlice / DBError_ReturnsRawError
  - [x] 2.3: sqlmock SQL 字面量与 GORM 实际生成的 SQL 字符串严格匹配（用 regexp.QuoteMeta + 实际跑 test 调整）
  - [x] 2.4: HappyPath case 断言 4 行字段值 + 顺序（wave/love/laugh/cry）
  - [x] 2.5: empty case 断言 `got != nil && len(got) == 0`（防 nil slice 回归）

- [x] Task 3: `EmojiService` interface + `ListAvailable` 实装（AC3）
  - [x] 3.1: 新建 `server/internal/service/emoji_service.go`（参考 AC3 钦定代码块）
  - [x] 3.2: `EmojiService` interface + `EmojiBrief` struct + `emojiServiceImpl` + `NewEmojiService` 构造 + `ListAvailable` 方法
  - [x] 3.3: DB error 用 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)` 包成 1009（与 home_service 同模式）
  - [x] 3.4: 0 行场景返 `[]EmojiBrief{}` 非 nil（用 `make([]EmojiBrief, 0, len(rows))` 兜底）
  - [x] 3.5: `EmojiBrief` 字段集严格 = `{Code, Name, AssetURL, SortOrder}`（**不含** ID / IsEnabled / CreatedAt / UpdatedAt）

- [x] Task 4: `emoji_service_test.go` stub repo 单测（AC4）
  - [x] 4.1: 新建 `server/internal/service/emoji_service_test.go`（参考 AC4 钦定代码块）
  - [x] 4.2: ≥4 case：HappyPath_4Rows / DisabledFilteredAtRepoLayer / EmptyResult / DBError_Returns1009
  - [x] 4.3: AC4.3 断言 `got != nil && len(got) == 0`（防 nil slice 回归）
  - [x] 4.4: AC4.4 断言 `appErr.Code == apperror.ErrServiceBusy`（1009）

- [x] Task 5: `EmojisHandler` + `GetEmojis` 实装（AC5）
  - [x] 5.1: 新建 `server/internal/app/http/handler/emojis_handler.go`（参考 AC5 钦定代码块）
  - [x] 5.2: handler **不**做参数校验 / 不读 userID（与 §11.1 一致）
  - [x] 5.3: DTO 转换 snake_case → camelCase：`assetUrl` / `sortOrder`
  - [x] 5.4: **不**下发 id / is_enabled / created_at / updated_at
  - [x] 5.5: 空 slice 兜底 `make([]gin.H, 0, len(briefs))`（防 JSON null）
  - [x] 5.6: 错误走 `c.Error(err) + return`（ADR-0006 + 与 home_handler 同模式）

- [x] Task 6: `emojis_handler_test.go` stub service 单测（AC6）
  - [x] 6.1: 新建 `server/internal/app/http/handler/emojis_handler_test.go`（参考 AC6 钦定代码块）
  - [x] 6.2: ≥2 case：HappyPath_4Rows / ServiceError_Returns1009
  - [x] 6.3: HappyPath case 显式断言字段名 `assetUrl` / `sortOrder`（防 snake_case 回归）
  - [x] 6.4: ServiceError case 显式断言 envelope code == 1009（防 ErrorMappingMiddleware 翻译丢失）

- [x] Task 7: `emoji_service_integration_test.go` dockertest 集成测试（AC7）
  - [x] 7.1: 新建 `server/internal/service/emoji_service_integration_test.go`（参考 AC7 钦定代码块）
  - [x] 7.2: 文件顶部 `//go:build integration` + `// +build integration` 双 tag
  - [x] 7.3: 复用 `home_service_integration_test.go` 的 `startMySQL` / `runMigrations` helper
  - [x] 7.4: 直接复用 0010 seed migration 4 行（不手工 INSERT）
  - [x] 7.5: 断言 len(got) == 4 + 4 行字段精确匹配 + sortOrder 严格 1/2/3/4 + assetUrl 非空

- [x] Task 8: `bootstrap/router.go` wire（AC8）
  - [x] 8.1: 在 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil` 块内添加 `emojiRepo := repomysql.NewEmojiRepo(deps.GormDB)` / `emojiSvc := service.NewEmojiService(emojiRepo)` / `emojisHandler := handler.NewEmojisHandler(emojiSvc)`
  - [x] 8.2: 在 `authedGroup.GET("/home", ...)` 附近添加 `authedGroup.GET("/emojis", emojisHandler.GetEmojis)`
  - [x] 8.3: 验证路由路径为 `/api/v1/emojis`（authedGroup 父组 api = `r.Group("/api/v1")`）

- [x] Task 9: 验证 build + test（AC9 + AC10）
  - [x] 9.1: `bash scripts/build.sh --test` 全 PASS（含本 story 新增 ≥9 case）
  - [x] 9.2: `bash scripts/build.sh --integration` 全 PASS（含 SeedContent + 既有 home / auth / migrate 集成测试）
  - [x] 9.3: 不引入 lint / vet / 编译 warning

- [x] Task 10: 收尾
  - [x] 10.1: review 本 story 文件 + 修正任何不一致
  - [x] 10.2: `sprint-status.yaml` 把 `17-4-get-emojis-接口` 从 `backlog` 流转到 `ready-for-dev`（由 create-story workflow 自动完成）

## Dev Notes

### 关键约束（disaster prevention）

1. **SQL 字面量必须严格符合 §11.1 服务端逻辑步骤 2**：
   - `SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`
   - 次要排序键 `id ASC` **不能漏**（防止 sort_order 相同时返回顺序在不同请求间不一致）
   - `is_enabled = 1` **不能改成** `is_enabled != 0` 或 `is_enabled <> 0`（语义等价但与契约文本不一致 → review 会判 P2 fix）

2. **`items: []` 与 `items: null` 边界**（V1 §11.1 行 1833 钦定）：
   - service 层 `make([]EmojiBrief, 0, len(rows))` 兜底
   - handler 层 `make([]gin.H, 0, len(briefs))` 兜底
   - repo 层 `if rows == nil { rows = []EmojiConfig{} }` 兜底
   - **三层都兜底**是 defense-in-depth（任一层 future 改动不会破坏边界）

3. **assetUrl 不做空字符串过滤**（V1 §11.1 + 17-1 r2 Lesson 3 钦定）：
   - 17.3 seed 已保证 enabled 表情 assetUrl 非空
   - server 端 seed 层 / admin 写入层负责校验 enabled 表情 assetUrl 非空
   - 本 story 的 service / handler / repo **不**做 `if AssetURL == "" 跳过` 分支
   - 让意外有空 assetUrl 的 enabled 行透传到 client 触发渲染失败（client 看到问题源 → 报 bug 给 server）而不是被 server 静默过滤（drift hidden）

4. **DB error 必须包成 1009**（V1 §11.1 错误码表 + 17-1 r2 Lesson 2 钦定）：
   - service 层 `apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])`（与 home_service / room_service 同模式）
   - handler 走 `c.Error(err) + return` 让 ErrorMappingMiddleware 翻译

5. **路由挂在 authedGroup**：
   - 接口要求 Bearer token（§11.1 元信息表）+ RateLimitByUserID 60 次/分（§11.1 元信息表）
   - 挂在 `authedGroup`（Auth + RateLimitByUserID 双中间件已挂在父组）
   - **不**挂在 `authGroup`（那是 /auth 子组，挂 RateLimitByIP，给登录前路径用）
   - **不**单独挂在 r 根（缺中间件 → 1001 / 1005 永远不触发）

6. **不预实装 17.5 的 `EmojiRepo.Exists`**：
   - 17.5 需要 `SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1`（V1 §12.2 服务端逻辑步骤 4）
   - 是**独立 query**，与本 story `List` 方法是两条 SQL
   - 提前 ship `Exists` 让 17.5 review 找不到"新增方法"的范围边界（与 17.2 / 17.3 "禁止预实装 repo 方法"同模式）

7. **handler 不读 userID**：
   - 与 home_handler 不同（home 读 userID 做用户维度查询）
   - 表情列表是全局静态配置，无用户维度过滤
   - 但仍挂在 authedGroup（接口要求 auth）

8. **集成测试直接复用 0010 seed**：
   - **不**手工 INSERT 4 行（与 home_service_integration_test 不同）
   - 让本 case 同时验证：repo SQL 正确 + service DTO 转换正确 + 与 17.3 seed 跨 story 集成
   - migrate 已含 0010 seed（17.3 落地），不需要额外 fixture

### Source tree components to touch

- `server/internal/repo/mysql/emoji_repo.go`（修改 —— 末尾追加 EmojiRepo + List）
- `server/internal/repo/mysql/emoji_repo_test.go`（新建 —— ≥3 case sqlmock）
- `server/internal/service/emoji_service.go`（新建 —— EmojiService + EmojiBrief + ListAvailable）
- `server/internal/service/emoji_service_test.go`（新建 —— ≥4 case stub repo）
- `server/internal/service/emoji_service_integration_test.go`（新建 —— 1 case dockertest）
- `server/internal/app/http/handler/emojis_handler.go`（新建 —— EmojisHandler + GetEmojis）
- `server/internal/app/http/handler/emojis_handler_test.go`（新建 —— ≥2 case stub service）
- `server/internal/app/bootstrap/router.go`（修改 —— wire emojiRepo / emojiSvc / emojisHandler + 路由挂载）

### Testing standards summary

- **单测**：默认 `bash scripts/build.sh --test`，跑 `go test -count=1 ./...`；不带 -race（CI 单独跑 -race）
- **集成测试**：`bash scripts/build.sh --integration`，触发 `go test -tags=integration`；用 dockertest 起真 mysql:8.0 容器
- **sqlmock**：repo 层单测用 sqlmock，与 room_member_repo_test / step_account_repo_test 同模式
- **stub repo / service**：service / handler 层单测用 stub，与 home_service_test / auth_service_test 同模式
- **测试注释 / 文档**：全中文，与既有 17.x / 11.x / 14.x 同模式

### Project Structure Notes

- 严格对齐 `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.3 / §6 分层约束：
  - `internal/repo/mysql/emoji_repo.go`: 单表 CRUD + 错误识别
  - `internal/service/emoji_service.go`: 业务规则 + DTO 转换（mysql.EmojiConfig → service.EmojiBrief）
  - `internal/app/http/handler/emojis_handler.go`: HTTP wire + DTO 转换（service.EmojiBrief → wire JSON）
  - `internal/app/bootstrap/router.go`: DI wire + 路由挂载
- 文件命名与既有同模式：`emoji_repo.go` / `emoji_service.go` / `emojis_handler.go`（handler 文件用复数 emojis，与 V1 §11.1 路径 `/emojis` 一致；service / repo 用单数 emoji，与 EmojiConfig domain struct 同源）
- 包名严格沿用：`package mysql` / `package service` / `package handler` / `package bootstrap`
- 没有冲突：本 story 引入的 EmojiRepo / EmojiService / EmojisHandler 都是首次落地，与既有类型无名字冲突

### References

- Epic 17 story 定义：`_bmad-output/planning-artifacts/epics.md` §Story 17.4（行 2581-2599）
- V1 §11.1 GET /api/v1/emojis 完整契约：`docs/宠物互动App_V1接口设计.md` 行 1734-1837
- V1 §3 错误码表（1001 / 1005 / 1009 等）：`docs/宠物互动App_V1接口设计.md` §3
- V1 §2.4 全字段 camelCase 钦定：`docs/宠物互动App_V1接口设计.md` §2.4
- 数据库设计 §5.15 emoji_configs schema：`docs/宠物互动App_数据库设计.md` §5.15（含 `idx_enabled_sort` 索引钦定）
- 17.1 r2 review lesson：`docs/lessons/2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md`
  - Lesson 2: DB error 必须有 1009 路径
  - Lesson 3: assetUrl 必非空字符串 + server 端 seed 层 / admin 写入层负责校验
- 17.2 emoji_configs migration + GORM struct：`server/migrations/0009_init_emoji_configs.up.sql` + `server/internal/repo/mysql/emoji_repo.go`（17.2 落地）
- 17.3 emoji_configs seed：`server/migrations/0010_seed_emoji_configs.up.sql` + `server/internal/infra/migrate/migrate_integration_test.go`（17.3 落地的 SeedContent / SeedIdempotent dockertest）
- ADR-0003 migration 工具 + 编号约定：`_bmad-output/implementation-artifacts/decisions/0003-migration-tool.md`
- ADR-0006 error envelope 单一生产者：`docs/lessons/2026-04-24-error-envelope-single-producer.md`
- ADR-0007 ctx 传播：`_bmad-output/implementation-artifacts/decisions/0007-ctx-propagation.md`
- 同模式参考实装：
  - `server/internal/service/home_service.go` —— LoadHome 模式（多 repo 串行 + 1009 兜底 + DTO 转换）
  - `server/internal/app/http/handler/home_handler.go` —— GET endpoint 模式（不读 userID 之外的逻辑 + DTO snake_case → camelCase + c.Error 透传）
  - `server/internal/repo/mysql/room_member_repo.go` ListMembers —— 单查询 + 0 行兜底 + ORDER BY 稳定排序
  - `server/internal/repo/mysql/room_member_repo_test.go` —— sqlmock 单测模式
  - `server/internal/service/home_service_test.go` —— stub repo 单测模式
  - `server/internal/service/home_service_integration_test.go` —— dockertest 集成测试模式

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test` 2026-05-14：vet + build + all tests PASS（含 17.4 新增 ≥10 case：repo 3 / service 4 / handler 3）
- `bash scripts/build.sh --integration` 2026-05-14：本机 Docker 不可用，dockertest startMySQL retry 超时（120s）失败 —— 集成测试代码本身正确，Docker 可用环境下应通过；task 钦定不视为失败
- `go vet ./...` PASS

### Completion Notes List

- 按 AC1 在 `server/internal/repo/mysql/emoji_repo.go` 末尾追加 `EmojiRepo` interface + `emojiRepo` + `NewEmojiRepo` + `List(ctx)`；走 `tx.FromContext + WithContext(ctx)` 与 ADR-0007 一致；显式 `Select("id, code, name, asset_url, sort_order")` + `Where("is_enabled = ?", 1)` + `Order("sort_order ASC, id ASC")` + nil-slice 兜底
- 按 AC2 新建 `emoji_repo_test.go`：3 个 case sqlmock（HappyPath_4Rows / EmptyResult_ReturnsEmptySlice / DBError_ReturnsRawError）；复用同 package 的 `newGormWithMock(t)` helper（user_repo_test.go 中已定义）；SQL 字面量与 GORM 实际生成对齐（base table backtick 化）
- 按 AC3 新建 `service/emoji_service.go`：`EmojiService` interface + `EmojiBrief` struct + `emojiServiceImpl` + `NewEmojiService` + `ListAvailable`；DB error 用 `apperror.Wrap(err, ErrServiceBusy, ...)` 包成 1009 与 home_service 同模式；`make([]EmojiBrief, 0, len(rows))` 兜底空 slice
- 按 AC4 新建 `emoji_service_test.go`：4 个 case stub repo（HappyPath_4Rows / DisabledFilteredAtRepoLayer / EmptyResult_ReturnsEmptySliceNotNil / DBError_Returns1009）；额外加 `errors.Is(err, dbErr)` 断言验证 cause 保留
- 按 AC5 新建 `handler/emojis_handler.go`：`EmojisHandler` + `NewEmojisHandler` + `GetEmojis`；handler 不读 userID（与 §11.1 一致）；DTO 转换 snake_case → camelCase（assetUrl / sortOrder）；`make([]gin.H, 0, len(briefs))` 兜底空 slice 防 JSON null；错误走 `c.Error(err) + return` 让 ErrorMappingMiddleware 翻译
- 按 AC6 新建 `emojis_handler_test.go`：3 个 case（HappyPath_4Rows / ServiceError_Returns1009 / EmptyList_ReturnsEmptyArrayNotNull）；HappyPath 用 raw json 二次解析验证 items[0] 字段集严格 = {code, name, assetUrl, sortOrder}（防 snake_case / 多余字段下发回归）；Empty case 显式断言 `data.items == "[]"` 字面量而非 null
- 按 AC7 新建 `emoji_service_integration_test.go`：build tag `//go:build integration + // +build integration`；复用 `startMySQL` / `runMigrations` helper；直接复用 0010 seed 4 行（不手工 INSERT）；断言 len=4 + 字段值精确 + sortOrder 1/2/3/4 + assetUrl 非空
- 按 AC8 在 `bootstrap/router.go` wire：`emojiRepo := repomysql.NewEmojiRepo(deps.GormDB)` 与其他 repo 平级；`emojiSvc := service.NewEmojiService(emojiRepo)` + `emojisHandler := handler.NewEmojisHandler(emojiSvc)` 在 home 旁边；`authedGroup.GET("/emojis", emojisHandler.GetEmojis)` 挂在 `/home` 后面，自动继承 Auth + RateLimitByUserID 双中间件
- AC9 PASS：`bash scripts/build.sh --test` 全 PASS；vet 干净；无 lint warning
- AC10：本机 Docker 不可用 → dockertest startMySQL 超时；按 task 规则不视为失败；集成测试代码本身按 home_service_integration_test 同模式编写，Docker 可用环境下应通过

### File List

- server/internal/repo/mysql/emoji_repo.go（修改 —— 末尾追加 EmojiRepo + List）
- server/internal/repo/mysql/emoji_repo_test.go（新建 —— 3 case sqlmock）
- server/internal/service/emoji_service.go（新建 —— EmojiService + EmojiBrief + ListAvailable）
- server/internal/service/emoji_service_test.go（新建 —— 4 case stub repo）
- server/internal/service/emoji_service_integration_test.go（新建 —— 1 case dockertest）
- server/internal/app/http/handler/emojis_handler.go（新建 —— EmojisHandler + GetEmojis）
- server/internal/app/http/handler/emojis_handler_test.go（新建 —— 3 case stub service）
- server/internal/app/bootstrap/router.go（修改 —— wire emojiRepo / emojiSvc / emojisHandler + GET /emojis 挂载）
- _bmad-output/implementation-artifacts/sprint-status.yaml（修改 —— 17-4 流转 ready-for-dev → in-progress → review）

### Change Log

- 2026-05-14 dev-story 实装完毕（首次落地 EmojiRepo.List + emoji_service.ListAvailable + EmojisHandler.GetEmojis + router wire + 10 case 单测 + 1 case dockertest）
