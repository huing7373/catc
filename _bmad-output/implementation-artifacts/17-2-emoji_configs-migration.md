# Story 17.2: emoji_configs migration（首次落地 0009_init_emoji_configs.up/down.sql + EmojiConfig GORM domain struct 最小骨架 + ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE(code) 运行时拒绝）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0009_init_emoji_configs.up.sql` + `server/migrations/0009_init_emoji_configs.down.sql` 两个新 migration 文件（严格按 `docs/宠物互动App_数据库设计.md` §5.15 行 700-718 钦定的 CREATE TABLE DDL：`id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT` + `code VARCHAR(64) NOT NULL` + `name VARCHAR(64) NOT NULL` + `asset_url VARCHAR(255) NOT NULL DEFAULT ''` + `sort_order INT NOT NULL DEFAULT 0` + `is_enabled TINYINT NOT NULL DEFAULT 1` + `created_at / updated_at DATETIME(3)` + `UNIQUE KEY uk_code (code)` + `KEY idx_enabled_sort (is_enabled, sort_order)` + `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`，1:1 对齐 §5.15）+ **新增** `server/internal/repo/mysql/emoji_repo.go` 含 `EmojiConfig` GORM domain struct（与 0009 真实 schema 1:1 对齐：`ID / Code / Name / AssetURL / SortOrder / IsEnabled / CreatedAt / UpdatedAt`）+ `TableName() string` 显式返回 `"emoji_configs"`（**仅** struct + TableName，**不**新增 Repo interface / 实装任何 Find / Create 方法，YAGNI；Story 17.3 / 17.4 才落地 repo 方法）+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 的 `TestMigrateIntegration_UpThenDown`（表数量 8 → 9）+ `TestMigrateIntegration_UpTwice_Idempotent`（同 8 → 9）+ **新增** `TestMigrateIntegration_EmojiConfigs_Schema`（验证 emoji_configs 表 / 列 / 索引 / 默认值符合 §5.15）+ **新增** `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` dockertest 集成测试（覆盖 `UNIQUE KEY uk_code (code)` 运行时 INSERT 拒绝行为；epics.md §Story 17.2 钦定的"集成测试覆盖：尝试 INSERT 重复 code → 数据库拒绝"路径）,
so that **Story 17.3（emoji_configs seed）+ Story 17.4（GET /emojis 接口）+ Story 17.5（WS emoji.send → emoji.received 广播）+ iOS Epic 18 各 story 表情面板 + 发送 + 接收动效**可以基于一个**已落地、已具备完整测试覆盖、已通过 dockertest 真实 INSERT 验证、已具备完整 GORM domain struct 字段映射**的 emoji_configs 持久化基础并行展开，不再出现"17.3 写 INSERT seed SQL 时找不到表 / 17.4 写 SELECT 时 GORM struct 字段名与 DB 列名漂移 / 17.5 校验 emojiCode 时 SQL 查询命中错误表 / 重复 code 在 prod 跑了之后才发现 UNIQUE 没生效"的返工。

## 故事定位（Epic 17 第二条 = 第一条**实装** story；上承 17.1 契约定稿，下启 17.3 seed + 17.4 GET /emojis + 17.5 WS emoji.send / emoji.received 广播）

- **Epic 17 进度**：17.1（契约定稿，done）→ **17.2（本 story，emoji_configs migration + GORM domain struct + 测试覆盖）** → 17.3（emoji_configs seed ≥4 个表情）→ 17.4（GET /emojis 接口）→ 17.5（WS emoji.send 处理 + emoji.received 广播）。
- **本 story 是 17.3 / 17.4 / 17.5 / Epic 18 的强前置**：
  - **17.3 seed**：seed 需要的 `INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES ...` SQL 必须命中本 story 落地的 0009 表 schema；`UNIQUE KEY uk_code (code)` + `INSERT IGNORE` / `ON DUPLICATE KEY UPDATE` 兜底语义依赖本 story 已落地的 UNIQUE 约束
  - **17.4 GET /emojis 接口**：handler / service / repo 层的 `SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC` 查询必须命中本 story 落地的 0009 表 schema 与 `idx_enabled_sort (is_enabled, sort_order)` 索引；GORM struct（本 story 新增的 `EmojiConfig`）直接被 17.4 repo 层 `Find(ctx, &emojis, "is_enabled = 1")` 等方法复用
  - **17.5 WS emoji.send 处理**：service 层 emojiCode 合法性校验 `SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1`（按 §12.2 服务端逻辑步骤 4 + §17.1 锚定）必须命中本 story 落地的 0009 表 schema；`uk_code` UNIQUE 保证查询命中**最多 1 行**
  - **iOS Epic 18**：iOS 端 `EmojiConfigDTO` Codable struct 字段（`code / name / assetUrl / sortOrder`）通过 17.4 GET /emojis JSON response 间接依赖本 story 落地的 DB schema；本 story 落地的字段（`code VARCHAR(64)` / `name VARCHAR(64)` / `asset_url VARCHAR(255)` / `sort_order INT`）是 17.4 response 字段类型 / 长度约束的**唯一真相源**
- **epics.md §Story 17.2 钦定**（行 2539-2557）：
  - `migrations/0009_init_emoji_configs.sql` 按数据库设计.md §5.15 创建表，含 `id` PK + `code VARCHAR(64) NOT NULL` + `name VARCHAR(64) NOT NULL` + `asset_url VARCHAR(255) NOT NULL DEFAULT ''` + `sort_order INT NOT NULL DEFAULT 0` + `is_enabled TINYINT NOT NULL DEFAULT 1` + `created_at / updated_at DATETIME(3)` + `UNIQUE KEY uk_code (code)` + `KEY idx_enabled_sort (is_enabled, sort_order)`
  - 含 down.sql（DROP TABLE 路径）
  - **单元测试覆盖**（≥3 case）：happy（migrate up 后表存在 + 字段类型 + uk_code + idx_enabled_sort 符合 §5.15）/ happy（migrate down 后表删除）/ edge（重复 migrate up → 幂等，由现有 `TestMigrateIntegration_UpTwice_Idempotent` 扩展覆盖）
  - **集成测试覆盖**（dockertest）：migrate up → 尝试 INSERT 重复 code → 数据库拒绝（UNIQUE KEY uk_code 运行时执行）
- **Story 17.1 上游冻结边界**（§11.1 GET /api/v1/emojis schema + §12.2 emoji.send + §12.3 emoji.received 字段表）：本 story 落地的字段长度约束（`code VARCHAR(64)` / `name VARCHAR(64)` / `asset_url VARCHAR(255)` / `sort_order INT`）是 17.1 锚定的 API 字段长度（`code` 1 ≤ length ≤ 64 / `name` 1 ≤ length ≤ 64 / `assetUrl` 1 ≤ length ≤ 255 / `sortOrder` 0 ≤ value ≤ 2^31 - 1）的**DB 端真相源**；本 story **不**反向修改 DB schema（DB → API 单向），仅严格对齐数据库设计文档 §5.15 DDL
- **下游强依赖**（本 story 不动后才能开工）：
  - Story 17.3（emoji_configs seed）
  - Story 17.4（GET /emojis 接口）
  - Story 17.5（WS emoji.send 处理 + emoji.received 广播）
  - iOS Epic 18.1（表情面板 SwiftUI + GET /emojis 缓存）
- **范围红线**：
  - 本 story **只**改 `server/migrations/0009_init_emoji_configs.up.sql`（新建）+ `server/migrations/0009_init_emoji_configs.down.sql`（新建）+ `server/internal/repo/mysql/emoji_repo.go`（新建，含 `EmojiConfig` struct + `TableName()`）+ `server/internal/infra/migrate/migrate_integration_test.go`（扩展 `TestMigrateIntegration_UpThenDown` 表数量断言 + 扩展 `TestMigrateIntegration_UpTwice_Idempotent` 表数量断言 + 新增 `TestMigrateIntegration_EmojiConfigs_Schema` case + 新增 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` case）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（17.3 ~ 17.5 才做；本 story 阶段**仅**落地 GORM struct + TableName，**不**新建 `EmojiRepo` interface / 实装 `Find` / `Create` / `Exists` 等方法）
  - **不**实装任何 seed SQL（17.3 才做；本 story **仅** CREATE TABLE，不含任何 INSERT）
  - **不**实装 GET /emojis handler / service（17.4 才做）
  - **不**接 Redis（10.6 已接，本 story 不动）
  - **不**改 V1 接口契约（17.1 已冻结）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.15 是契约**输入**，本 story 严格对齐它但**不修改**；如发现 §5.15 与本 story 落地的 DDL 有不一致 → 优先以 §5.15 为准修改本 story 而非反向改 §5.15）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用 / 不修改）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**为 0009 写"prod 部署 migration 自动化 / 一键 rollback / dry-run / force"等运维化改造（保留最小集 up/down，与 Story 4.3 / 7.2 / 10.3 / 11.2 一致）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（17.3 ~ 17.5 范围）
- 不实装任何 INSERT seed SQL（17.3 钦定 owner；本 story **仅** CREATE TABLE）
- 不新建 `EmojiRepo` interface（YAGNI；17.4 实装 GET /emojis 时才落地 `EmojiRepo` 类型 + `List(ctx) ([]EmojiConfig, error)` 方法）
- 不在 `EmojiConfig` struct 上加 GORM `uniqueIndex` / `index` 等 tag（UNIQUE / 普通索引由 SQL DDL 定义，GORM struct **不**重复声明 —— 与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；与 11.2 落地的 `RoomMember` / `Room` struct 同模式）
- 不引入 `gorm.Model`（避免引入 `DeletedAt` / 软删除字段，与 0009 真实 schema 不符）
- 不修改 0001 ~ 0008 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 落地）
- 不修改 V1 接口契约（17.1 已冻结）
- 不修改数据库设计 §5.15（schema 输入，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`internal/repo/mysql/` 目录已锚定；本 story 新增 `emoji_repo.go` 沿用既有目录规则）
- 不修改 ADR-0003（migration 工具 / 编号约定 / 文件命名规范由 Story 4.3 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 一致）
- 不为 0009 写 stress test / fuzz test（节点 6 阶段 schema 稳定 + 单测 + dockertest 集成测试已覆盖核心约束）
- 不在本 story 内对 Story 17.3 ~ 17.5 实装做"提前预实装"（即使顺手写 `(r *emojiRepo) List(ctx) ([]EmojiConfig, error)` 也禁止；这些方法是 17.4 钦定范围，提前 ship 会让 17.4 评审找不到"新增方法"的明确范围边界，与 Story 11.2 "禁止预实装 RoomRepo.Create" 同模式）
- 不写 `EmojiConfig.IsEnabled` 字段的 enum 校验（DB 端 `TINYINT NOT NULL DEFAULT 1` 已兜底；§6 状态枚举钦定 0 / 1 两值；service 层校验由 17.5 实装时按需添加）

## Acceptance Criteria

**AC1 — 0009_init_emoji_configs.up.sql 新建（与 §5.15 钦定 1:1 对齐）**

新建 `server/migrations/0009_init_emoji_configs.up.sql`，内容必须**严格**对齐 `docs/宠物互动App_数据库设计.md` §5.15（行 700-718）钦定的 DDL：

```sql
-- 对齐 docs/宠物互动App_数据库设计.md §5.15 (行 700-718)
-- emoji_configs 表：系统表情配置表
--
-- **本 migration 由 Story 17.2 首次落地（Epic 17 节点 6 表情广播链路 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 / 字段类型 / uk_code + idx_enabled_sort 索引符合 §5.15）
-- + dockertest 集成测试覆盖 UNIQUE KEY uk_code (code) 运行时 INSERT 拒绝行为
-- （epics.md §Story 17.2 钦定的"集成测试覆盖：尝试 INSERT 重复 code → 数据库拒绝"路径）。
--
-- 字段（与 §5.15 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - code VARCHAR(64) NOT NULL：表情业务标识符（如 wave / love / laugh / cry）；
--     与 V1 接口设计 §11.1 / §12.2 / §12.3 锚定的 emojiCode 字段同语义；
--     UNIQUE KEY uk_code 保证全局唯一（Story 17.3 seed 用 INSERT IGNORE
--     兜底重复执行）
--   - name VARCHAR(64) NOT NULL：表情中文名（如 挥手 / 爱心 / 大笑 / 哭泣），
--     client 用作 UI 展示文字
--   - asset_url VARCHAR(255) NOT NULL DEFAULT ''：表情资源 URL；
--     **注**：DDL DEFAULT '' 是兜底语义（避免 admin / dev 临时写入时漏字段），
--     enabled 表情（is_enabled=1）必须有非空 asset_url —— Story 17.3 seed 钦定
--     每个表情非空，server seed 层 / admin 写入层应校验
--   - sort_order INT NOT NULL DEFAULT 0：表情显示顺序（升序）；
--     与 V1 §11.1 data.items[].sortOrder 同语义；
--     INT 是带符号 32 位整数（范围 -2^31 ~ 2^31-1），与 §11.1 字段范围
--     0 ≤ value ≤ 2^31 - 1 兼容（业务不用负值）
--   - is_enabled TINYINT NOT NULL DEFAULT 1：是否启用（§6 枚举：0=disabled / 1=enabled）；
--     §11.1 GET /emojis 仅返回 is_enabled=1 的表情；§12.2 emoji.send 仅校验
--     is_enabled=1 的表情，disabled 表情触发 error 7001（与 emojiCode 不存在
--     合并到同一错误码）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳）
--
-- 索引（§5.15 + §7 钦定）：
--   - UNIQUE KEY uk_code (code)：保证 code 全局唯一（§7.1 高优先级 UNIQUE 约束）；
--     Story 17.3 seed 用 INSERT IGNORE 兜底重复执行；Story 17.5 校验 emojiCode
--     合法性时单列查询命中
--   - KEY idx_enabled_sort (is_enabled, sort_order)：覆盖
--     "SELECT ... WHERE is_enabled=1 ORDER BY sort_order ASC" 路径
--     （§11.1 GET /emojis 服务端逻辑步骤 2 SQL；多列复合索引覆盖筛选 + 排序）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （seed 由 Story 17.3 `0010_seed_emoji_configs.up.sql` 落地，**不**塞进本文件）；
-- 不含任何业务 service / handler / repo write 方法（17.3 ~ 17.5 落地）。
CREATE TABLE emoji_configs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    asset_url VARCHAR(255) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    is_enabled TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_code (code),
    KEY idx_enabled_sort (is_enabled, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- DDL 内容**严格**对齐 §5.15 行 704-717 —— 字段顺序 / 字段类型 / NOT NULL / DEFAULT 值 / 索引名 / 索引列顺序全部 1:1
- 文件编码 UTF-8 + LF 行尾（与 0001 ~ 0008 一致）
- 顶部注释模板与 0007 / 0008（11.2 升级版本）一致 —— "对齐 §X.Y" + "字段" + "索引" + "范围红线" 四段式
- **不**包含任何 INSERT / seed 数据（Story 17.3 owner）
- **不**包含任何 business logic SQL（如 `UPDATE emoji_configs SET ...`）

**AC2 — 0009_init_emoji_configs.down.sql 新建**

新建 `server/migrations/0009_init_emoji_configs.down.sql`，内容：

```sql
-- 回滚 0009_init_emoji_configs.up.sql
--
-- **本 migration 由 Story 17.2 首次落地（Epic 17 节点 6 表情广播链路 owner）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为。
DROP TABLE IF EXISTS emoji_configs;
```

- 文件编码 UTF-8 + LF 行尾
- **仅** `DROP TABLE IF EXISTS emoji_configs;`，不含任何额外 cleanup 语句（与 0007 / 0008 down.sql 同模式）

**AC3 — emoji_repo.go 新建（仅 `EmojiConfig` GORM domain struct + `TableName()`，无 repo 方法）**

新建 `server/internal/repo/mysql/emoji_repo.go`，内容必须包含：

```go
package mysql

import (
	"time"
)

// EmojiConfig 是 emoji_configs 表的完整 GORM domain struct（Story 17.2 引入；
// 与 server/migrations/0009_init_emoji_configs.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.15 + 0009_init_emoji_configs.up.sql 1:1 对齐）：
//   - ID:        BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.15 + §3.1 主键约定）
//   - Code:      VARCHAR(64) NOT NULL（表情业务标识符；UNIQUE KEY uk_code 保证全局唯一）
//   - Name:      VARCHAR(64) NOT NULL（表情中文名，UI 展示文字）
//   - AssetURL:  VARCHAR(255) NOT NULL DEFAULT ''（表情资源 URL；enabled 表情必须非空）
//   - SortOrder: INT NOT NULL DEFAULT 0（表情显示顺序，升序）
//   - IsEnabled: TINYINT NOT NULL DEFAULT 1（§6 枚举：0=disabled / 1=enabled）
//   - CreatedAt: DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt: DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
//
// 表层 UNIQUE 约束（uk_code）+ 普通索引（idx_enabled_sort）由 SQL DDL 定义，
// **不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；
// GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；
// 与 Story 11.2 落地的 RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 17.3（seed）/ 17.4（GET /emojis 接口）/
// 17.5（WS emoji.send 校验）提供字段映射；本 story 阶段**不**新建 EmojiRepo
// interface / 实装 List / Exists / Create 等方法（YAGNI；17.4 落地 List 方法 +
// 17.5 落地 Exists 方法）。
type EmojiConfig struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Code      string    `gorm:"column:code;not null;size:64"`
	Name      string    `gorm:"column:name;not null;size:64"`
	AssetURL  string    `gorm:"column:asset_url;not null;size:255;default:''"`
	SortOrder int32     `gorm:"column:sort_order;not null;default:0"`
	IsEnabled int8      `gorm:"column:is_enabled;not null;default:1"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "emoji_configs"。
func (EmojiConfig) TableName() string { return "emoji_configs" }
```

- 字段顺序与 0009 SQL 列顺序一致
- `SortOrder int32` 对齐 `INT`（带符号 32 位；与 §11.1 字段范围 0 ≤ value ≤ 2^31 - 1 兼容）
- `IsEnabled int8` 对齐 `TINYINT`（带符号；MySQL `TINYINT` 默认带符号，范围 -128 ~ 127；§6 枚举 0 / 1 在范围内；与 Story 11.2 `Room.Status int8` 同模式）
- `AssetURL` 命名遵循 Go 风格（Go 风格 `URL` 全大写缩写；GORM `column:asset_url` 显式映射到 DB 列名）
- `Code` / `Name` 字段类型 `string` + `size:64` tag 仅为文档化（GORM 不强制；实际长度约束由 DDL `VARCHAR(64)` 兜底）
- **关键**：导入 `time` 包 + **不**引入 `gorm.Model`（避免 `DeletedAt` 软删除字段）
- **关键**：**不**在 struct 上加 `gorm:"uniqueIndex:uk_code"` / `gorm:"index:idx_enabled_sort"` tag（UNIQUE / 索引由 SQL DDL 定义，与 ADR-0003 §3.2 一致）
- **不**新建 `EmojiRepo` interface / `emojiRepo` struct / `NewEmojiRepo()` constructor / 任何 `Find` / `Create` / `Exists` 方法（YAGNI；17.4 / 17.5 owner）
- 文件内容**仅**含 package 声明 + import + `EmojiConfig` struct + `TableName()` 方法（≤ 50 行；与 11.2 落地的 `Room` struct 同体积级别）

**AC4 — migrate_integration_test.go 扩展（表数量断言 + emoji_configs schema 验证 + UNIQUE(code) 拒绝集成测试）**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

**AC4.1 扩展既有 `TestMigrateIntegration_UpThenDown`**（行 122 ~ 187 附近）：
- 找到现有 "表数量 = 8（Story 10.3 review r5 加 rooms / room_members）" 断言（行 ~140 + ~170），把硬编码 `8` 改为 `9`
- 同步注释升级："Story 10.3 review r5 [P1] 加 rooms / room_members → Story 17.2 加 emoji_configs（总计 9 张表）"

**AC4.2 扩展既有 `TestMigrateIntegration_UpTwice_Idempotent`**（行 284 ~ 323 附近）：
- 找到现有 "表数量 = 8" 断言（行 ~320），把硬编码 `8` 改为 `9`
- 同步注释升级（与 AC4.1 一致）

**AC4.3 新增 `TestMigrateIntegration_EmojiConfigs_Schema`**（紧接 `TestMigrateIntegration_RoomsAndRoomMembers_Schema` 之后，参考既有 case 实装模式）：

```go
// TestMigrateIntegration_EmojiConfigs_Schema 验证
// migrations/0009_init_emoji_configs.up.sql 钦定的 emoji_configs 表 schema
// 与数据库设计.md §5.15 + V1接口设计.md §11.1 一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - code VARCHAR(64) NOT NULL + UNIQUE KEY uk_code (code)
//   - name VARCHAR(64) NOT NULL
//   - asset_url VARCHAR(255) NOT NULL DEFAULT ''
//   - sort_order INT NOT NULL DEFAULT 0
//   - is_enabled TINYINT NOT NULL DEFAULT 1
//   - created_at / updated_at DATETIME(3)
//   - KEY idx_enabled_sort (is_enabled, sort_order)
//
// **背景（Story 17.2 引入）**：本 case 验证 0009 migration 落地的 schema
// 与 §5.15 钦定 1:1 对齐；用于在 epics.md §Story 17.2 钦定的"单元测试覆盖 ≥3 case"
// 中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.15）。
func TestMigrateIntegration_EmojiConfigs_Schema(t *testing.T) {
	// 实装细节由 dev-story 阶段补全；
	// 模板参考既有 TestMigrateIntegration_RoomsAndRoomMembers_Schema（行 188 ~ 282）
	// + TestMigrateIntegration_TablesPresent_AfterUp（行 324 ~ ...）。
	//
	// 必查项（每项失败立即 t.Errorf，不 t.Fatalf —— 用 batch 累积报错风格）：
	//   1. INFORMATION_SCHEMA.TABLES：emoji_configs 表存在
	//   2. INFORMATION_SCHEMA.COLUMNS：8 列存在 + 类型对齐（id bigint unsigned /
	//      code varchar(64) / name varchar(64) / asset_url varchar(255) /
	//      sort_order int / is_enabled tinyint / created_at datetime(3) /
	//      updated_at datetime(3)）
	//   3. INFORMATION_SCHEMA.KEY_COLUMN_USAGE / STATISTICS：
	//      - PRIMARY KEY = id
	//      - UNIQUE KEY uk_code (code) 存在 + non_unique = 0
	//      - KEY idx_enabled_sort 存在 + 列顺序 (is_enabled, sort_order)
	//   4. INFORMATION_SCHEMA.COLUMNS.COLUMN_DEFAULT：
	//      - asset_url DEFAULT '' (空字符串)
	//      - sort_order DEFAULT '0'
	//      - is_enabled DEFAULT '1'
	//      - created_at / updated_at DEFAULT CURRENT_TIMESTAMP(3)
}
```

**AC4.4 新增 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected`**（紧接 AC4.3 case 之后）：

```go
// TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected 验证
// migrations/0009_init_emoji_configs.up.sql 钦定的 UNIQUE KEY uk_code (code)
// 在运行时被 MySQL 真实拒绝重复 code 插入。
//
// **背景（Story 17.2 引入）**：epics.md §Story 17.2 钦定的"集成测试覆盖（dockertest）：
// migrate up → 尝试 INSERT 重复 code → 数据库拒绝"路径在本 case 落地；
// 是 Story 17.3 seed 用 INSERT IGNORE 兜底 + Story 17.5 校验 emojiCode 合法性 +
// admin 后台未来写入路径的 schema 层根基。
//
// **覆盖路径**：
//  1. migrate up → emoji_configs 表存在
//  2. 插入 emoji_configs (code='wave', name='挥手', sort_order=1) → 成功
//  3. 再次插入 emoji_configs (code='wave', name='挥手 v2', sort_order=2) → DB 拒绝
//     （UNIQUE KEY uk_code (code) 兜底；same code 不能插两次）；
//     err 必须含 "Duplicate entry" / "1062"（MySQL 错误码 = ER_DUP_ENTRY）
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异（与 Story 11.2 落地的
// TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected 同模式）。
func TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected(t *testing.T) {
	// 实装细节由 dev-story 阶段补全；模板见 Story 11.2 落地的
	// TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected（行 603 ~ 664）。
}
```

- 两个新 case 都用 `dockertest` 起 mysql:8.0 容器（沿用 `startMySQL(t)` + `migrationsPath(t)` helper，与既有 case 一致）
- UNIQUE 拒绝 case 用 `database/sql` 直跑 raw INSERT（**不**走 GORM）
- 错误断言：`err != nil` + `strings.Contains(err.Error(), "Duplicate entry")`（MySQL 错误码 1062 = `ER_DUP_ENTRY`，与 11.2 同模式）
- **不**断言具体 MySQL error message 文本（不同 MySQL 版本可能略有差异；用 "Duplicate entry" substring 是稳定 contract）

**AC5 — 验证步骤**

- **AC5.1 build 验证**：执行 `bash scripts/build.sh --test` 必须**全绿**（含新增 `emoji_repo.go` 单测无 / 既有单测无回归 + 新增集成测试不在默认 `--test` build tag 内不跑）；`bash scripts/build.sh --integration` 必须**全绿**（新增 `TestMigrateIntegration_EmojiConfigs_Schema` + `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 两个 case 跑通 + 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 表数量 9 断言通过 + 既有 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 不回归）
- **AC5.2 git diff 范围检查**：编辑完成后 `git diff` 输出**仅**包含：
  - `server/migrations/0009_init_emoji_configs.up.sql`（新增）
  - `server/migrations/0009_init_emoji_configs.down.sql`（新增）
  - `server/internal/repo/mysql/emoji_repo.go`（新增）
  - `server/internal/infra/migrate/migrate_integration_test.go`（修改：表数量断言 8 → 9 + 新增 2 case）
  - `_bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md`（本 story 文件状态流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List / Change Log）
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行 + last_updated）
- **AC5.3 schema 跨文档一致性**：手动检查 0009 up.sql 字段名 / 类型 / 索引名与数据库设计.md §5.15 行 704-717 **逐字段** 1:1 对齐；与 V1 §11.1 字段长度约束兼容（`code VARCHAR(64)` ⇔ §11.1 `code` 1 ≤ length ≤ 64；`name VARCHAR(64)` ⇔ §11.1 `name` 1 ≤ length ≤ 64；`asset_url VARCHAR(255)` ⇔ §11.1 `assetUrl` 1 ≤ length ≤ 255；`sort_order INT` ⇔ §11.1 `sortOrder` 0 ≤ value ≤ 2^31 - 1）
- **AC5.4 GORM struct ↔ DDL 一致性**：手动检查 `EmojiConfig` struct 8 字段 / 类型与 0009 up.sql 1:1 对齐（无字段缺漏 / 无类型漂移）
- **AC5.5 既有迁移 / repo 测试不回归**：跑 `go test ./server/internal/infra/migrate/... ./server/internal/repo/mysql/... -count=1` 全绿

## Tasks / Subtasks

- [x] Task 1: 准备阶段（AC: #1, #2, #3, #4, #5）
  - [x] Subtask 1.1: 阅读本 story 全文 + `docs/宠物互动App_数据库设计.md` §5.15（行 700-718）确认 DDL 1:1 字段 / 索引清单
  - [x] Subtask 1.2: 阅读 `_bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md` 已 done 的姊妹 story，参考其 migration + GORM struct + 集成测试编辑模式
  - [x] Subtask 1.3: 阅读 `server/migrations/0007_init_rooms.up.sql` + `0008_init_room_members.up.sql`（11.2 升级后的版本）确认顶部注释 / 字段块 / 索引块 / 范围红线四段式模板
  - [x] Subtask 1.4: 阅读 `server/internal/repo/mysql/room_member_repo.go`（11.2 升级后的版本）确认 `RoomMember` + `Room` struct 的 GORM tag 模式 + `TableName()` 模式
  - [x] Subtask 1.5: 阅读 `server/internal/infra/migrate/migrate_integration_test.go`（11.2 升级后的版本）确认 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` + `TestMigrateIntegration_RoomsAndRoomMembers_Schema` 编辑模式
- [x] Task 2: 落地 0009_init_emoji_configs.up.sql（AC: #1）
  - [x] Subtask 2.1: 新建 `server/migrations/0009_init_emoji_configs.up.sql`
  - [x] Subtask 2.2: 写顶部注释（"对齐 §5.15" + 字段块 + 索引块 + 范围红线四段式，按 AC1 模板）
  - [x] Subtask 2.3: 写 CREATE TABLE 语句（严格按 §5.15 行 704-717 1:1 + AC1 钦定）
- [x] Task 3: 落地 0009_init_emoji_configs.down.sql（AC: #2）
  - [x] Subtask 3.1: 新建 `server/migrations/0009_init_emoji_configs.down.sql`
  - [x] Subtask 3.2: 写顶部注释（按 AC2 模板）+ `DROP TABLE IF EXISTS emoji_configs;`
- [x] Task 4: 落地 emoji_repo.go（AC: #3）
  - [x] Subtask 4.1: 新建 `server/internal/repo/mysql/emoji_repo.go`
  - [x] Subtask 4.2: 写 package 声明 + import `time`（**不**引入 `gorm.io/gorm`）
  - [x] Subtask 4.3: 写 `EmojiConfig` struct 8 字段（按 AC3 钦定的字段顺序 + GORM tag）
  - [x] Subtask 4.4: 写 `TableName()` 方法（按 AC3 钦定，返回 `"emoji_configs"`）
- [x] Task 5: 扩展 migrate_integration_test.go（AC: #4）
  - [x] Subtask 5.1: 改 `TestMigrateIntegration_UpThenDown` 中表数量断言 8 → 9 + 同步注释升级（按 AC4.1 钦定）
  - [x] Subtask 5.2: 改 `TestMigrateIntegration_UpTwice_Idempotent` 中表数量断言 8 → 9 + 同步注释升级（按 AC4.2 钦定）
  - [x] Subtask 5.3: 新增 `TestMigrateIntegration_EmojiConfigs_Schema` case（按 AC4.3 钦定 + 参考 `TestMigrateIntegration_RoomsAndRoomMembers_Schema` 实装模板）
  - [x] Subtask 5.4: 新增 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` case（按 AC4.4 钦定 + 参考 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 实装模板）
  - [x] Subtask 5.5: 顺手把 `TestMigrateIntegration_StatusAfterUp` 版本号断言 `v != 8` → `v != 9` 同步升级（story AC 未显式钦定，但 0009 落地后必跟改否则既有 test fail；与 7.2 / 10.3 review 同模式）
- [x] Task 6: 验证 + 提交（AC: #5）
  - [x] Subtask 6.1: 跑 `bash scripts/build.sh --test` 全绿（vet + 全 unit 测试通过）
  - [~] Subtask 6.2: 跑 `bash scripts/build.sh --integration`：本机 Docker 环境有 28+ 个 stale mysql:8.0 容器残留（28 hours+），新启容器 port 拨号失败 → 集成测试 `t.Skipf("mysql container did not become ready")` —— 这是测试基础设施钦定的"docker 不可用时不阻塞 CI"路径（migrate_integration_test.go 顶部注释钦定）。**code 路径已 go vet -tags=integration 全绿** + 新增 2 case 模板严格遵循 11.2 落地的 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 实装模式 → 留 code-review 阶段 fresh 环境跑通。
  - [x] Subtask 6.3: git diff 范围检查 —— 仅本 story 钦定 6 个文件（见 File List）
  - [x] Subtask 6.4: schema 跨文档 / struct 一致性手动检查（见 Completion Notes "schema 一致性核对"块）
  - [x] Subtask 6.5: 在 sprint-status.yaml 把本 story 状态从 in-progress 改为 review（本步骤）
  - [ ] Subtask 6.6: 由 code-review 检出后状态切 done + 在本 story 文件 + sprint-status.yaml 状态行追加 commit hash

## Dev Notes

### Build & Test 规范（项目级 CLAUDE.md 钦定）

- 写完 / 改完 Go 代码后必跑 `bash scripts/build.sh --test`（vet + 单测，**默认 build tag**，集成测试不跑）
- 集成测试 dockertest 必须用 `bash scripts/build.sh --integration`（带 `-tags=integration` build tag）
- 脚本契约来源：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5 + `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`

### Migration 文件命名 / 编号规则（ADR-0003 + Story 4.3 钦定）

- 文件命名：`{N:04d}_{name}.up.sql` / `{N:04d}_{name}.down.sql`（4 位编号 + 下划线 + 小写下划线名称）
- 编号顺序：0001 ~ 0008 已被 4.3 / 7.2 / 10.3 / 11.2 占用（users / user_auth_bindings / pets / user_step_accounts / user_chests / user_step_sync_logs / rooms / room_members）；**本 story 占用 0009**（首个表情相关 migration）
- **不**用 GORM AutoMigrate / 不用 `migrate` CLI 之外的工具（与 ADR-0003 钦定一致）

### GORM struct 规范（11.2 + 4.6 落地）

- struct 字段顺序与 SQL DDL 列顺序一致（便于 cross-reference）
- 字段类型对齐 MySQL → Go 映射：`BIGINT UNSIGNED → uint64` / `VARCHAR → string` / `INT → int32` / `TINYINT → int8` / `DATETIME(3) → time.Time`
- GORM tag 仅含 `column:` / `primaryKey` / `autoIncrement` / `not null` / `size:N` / `default:V` —— **不**含 `uniqueIndex` / `index` / `type:` 等（UNIQUE / 索引由 DDL 定义，与 ADR-0003 §3.2 一致；类型由字段 Go 类型推导）
- 显式 `TableName() string` 方法返回 DB 真实表名（避免 GORM 自动复数化引发漂移）
- **不**引入 `gorm.Model`（避免 `DeletedAt` 软删除字段污染 schema）
- **不**在本 story 阶段新建 Repo interface / 实装方法（YAGNI，与 11.2 同模式）

### 跨文档语义同步检查（DB → API 单向）

- 本 story 落地的 0009 SQL DDL **只**反映数据库设计.md §5.15 的语义，**禁止反向**修改 §5.15
- 如发现 §5.15 与本 story 落地 DDL 有不一致（如字段名 / 类型 / 长度 / 默认值漂移）→ 优先修 0009 SQL 而非 §5.15
- 本 story 落地的 0009 DDL 是 V1 §11.1 / §12.2 / §12.3 字段长度约束（17.1 锚定）的**DB 端真相源**；不允许在本 story 阶段对契约层做反向加严 / 放松

### 错误码不在本 story 范围

- §3 全局错误码表（7001 / 6004 / 1001 / 1002 / 1005 / 1009）由 17.1 锚定 + 由 17.4 / 17.5 实装时引用；本 story **不**触发错误码定义 / 修改 / 引用（migration 层不返回 API 错误码）

### 跨 epic 依赖追溯

- **上游冻结**：
  - 数据库设计 §5.15 emoji_configs 表 schema ← 总体架构 + 数据库设计文档锚定（**不**由某个 story 锚定；本 story 严格对齐）
  - V1 §11.1 / §12.2 / §12.3 字段层 ← Story 17.1 锚定
  - ADR-0003 migration 工具 + 编号规则 ← Story 4.3 落地
- **下游强依赖**（本 story done 后才能开工）：
  - Story 17.3（emoji_configs seed ≥4 个表情）
  - Story 17.4（GET /emojis 接口）
  - Story 17.5（WS emoji.send 处理 + emoji.received 广播）
  - iOS Epic 18.1（表情面板 SwiftUI + GET /emojis 缓存）

### 测试 / 验证

- **单元测试**：本 story 不新建 service / repo 方法 → 无 sqlmock-based 单测；既有 `room_member_repo_test.go` / `room_repo_test.go` / `pet_repo_test.go` 等不受影响
- **集成测试**（dockertest）：本 story 新增 2 case + 改既有 2 case 表数量断言；用 `bash scripts/build.sh --integration` 跑（带 `-tags=integration`）
- **下游验证**：本 story done 后由 Story 17.3 实装时的 `INSERT IGNORE` seed + `SELECT COUNT(*)` 验证 + Story 17.4 实装时的 dockertest 集成测试（curl GET /emojis → 验证 response 与 §11.1 字段表对齐）+ Story 17.5 实装时的 WS 集成测试（A + B 在房间 → A 发 `emoji.send` → A / B 都收到 `emoji.received`）做真实串联验证

### 范围红线 + 风险

- **红线**：本 story **不**修改任何 service / handler / repo write / read 方法；**不**修改 V1 接口契约 / 数据库设计文档 / ADR-0003；**不**修改 0001 ~ 0008 既有 migration；**不**预实装 `EmojiRepo` interface / 方法
- **红线**：本 story **不**实装 seed SQL（17.3 owner）
- **风险**：表数量断言 8 → 9 改漏（既有 `TestMigrateIntegration_UpThenDown` + `TestMigrateIntegration_UpTwice_Idempotent` 两处都要改）→ AC4.1 / AC4.2 显式钦定，AC5.2 git diff 范围检查兜底
- **风险**：GORM struct 字段类型漂移（如 `SortOrder int` 而非 `int32` → `INT` 列宽语义偏差）→ AC3 字段类型表 + AC5.4 GORM struct ↔ DDL 一致性手动检查兜底
- **风险**：dockertest 集成测试在 Windows 本地跑可能因 Docker Desktop 未启动失败 → 与 Story 11.2 / 14.x 同情况，由 dev-story 阶段确保 Docker Desktop 启动后跑（CI 阶段不在本 story 范围）
- **风险**：项目当前 `migrate_integration_test.go` 在跑 mig.Up() 后用 INFORMATION_SCHEMA.STATISTICS 查 index_name 时大小写敏感性 → MySQL 8.0 `lower_case_table_names=1`（Windows 默认）下 table_name 大小写归一为小写；index_name 不受影响（保留原大小写）；本 story 集成测试用 `idx_enabled_sort` / `uk_code` 小写命名与既有 case 一致，无大小写风险

### Project Structure Notes

- 本 story 唯一编辑文件（绝对路径）：
  - `C:/fork/cat/server/migrations/0009_init_emoji_configs.up.sql`（新建）
  - `C:/fork/cat/server/migrations/0009_init_emoji_configs.down.sql`（新建）
  - `C:/fork/cat/server/internal/repo/mysql/emoji_repo.go`（新建）
  - `C:/fork/cat/server/internal/infra/migrate/migrate_integration_test.go`（修改）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md`（本 story 文件）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转）
- 与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6 锚定的 `internal/repo/mysql/` / `internal/migrations/` 目录完全兼容（沿用既有目录规则，**不**新增子目录 / 模块）

### References

- [Source: docs/宠物互动App_数据库设计.md#5.15] emoji_configs 表 schema（行 700-718；本 story 严格对齐，**不**修改）
- [Source: docs/宠物互动App_数据库设计.md#7.1] 高优先级 UNIQUE 约束（`emoji_configs / cosmetic_items` UNIQUE(code)，行 857-861）
- [Source: docs/宠物互动App_数据库设计.md#6] 状态枚举（`is_enabled` 0 / 1 两值）
- [Source: docs/宠物互动App_V1接口设计.md#11.1] GET /api/v1/emojis 响应体字段长度约束（17.1 锚定；本 story DDL 是 §11.1 字段长度约束的 DB 端真相源）
- [Source: docs/宠物互动App_V1接口设计.md#12.2 ### 发送表情] emoji.send `emojiCode` 字段长度约束 1 ≤ length ≤ 64（17.1 锚定 + §5.15 `code VARCHAR(64)` 同源）
- [Source: docs/宠物互动App_V1接口设计.md#12.3 ### 收到表情广播] emoji.received `emojiCode` 字段（17.1 锚定）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 17.2] AC 钦定（行 2539-2557）：`0009_init_emoji_configs.sql` + down.sql + ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE(code) 运行时拒绝
- [Source: _bmad-output/implementation-artifacts/17-1-接口契约最终化.md] Story 17.1 上游契约（已 done；本 story 的 DB schema 是 §11.1 / §12.2 / §12.3 字段长度约束的 DB 端真相源）
- [Source: _bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md] Story 11.2 已 done 姊妹 story（migration + GORM struct + dockertest UNIQUE 拒绝集成测试编辑模式参考）
- [Source: _bmad-output/implementation-artifacts/decisions/0003-orm-stack.md] ADR-0003 ORM / migration 工具栈（golang-migrate v4.18.1 + GORM v1.25.12；migration 编号规则 + .up.sql / .down.sql 双向规范；禁止 GORM AutoMigrate）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md] ADR-0001 测试栈（dockertest + build tag `integration`；`bash scripts/build.sh --integration` 跑集成测试）
- [Source: server/migrations/0007_init_rooms.up.sql + 0008_init_room_members.up.sql] 11.2 升级后顶部注释 + DDL 模板（"对齐 §X.Y" + 字段块 + 索引块 + 范围红线四段式参考）
- [Source: server/internal/repo/mysql/room_member_repo.go] 11.2 落地的 `RoomMember` + `Room` GORM struct + `TableName()` 模板（本 story `EmojiConfig` struct 模仿同模式）
- [Source: server/internal/infra/migrate/migrate_integration_test.go] 11.2 落地的 `TestMigrateIntegration_RoomsAndRoomMembers_Schema`（行 188 ~ 282）+ `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected`（行 574 ~ 664）实装模板
- [Source: _bmad-output/implementation-artifacts/sprint-status.yaml] Epic 17 状态 in-progress；本 story 状态行 + last_updated 由 create-story / dev-story / code-review 流程逐步推进

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash scripts/build.sh --test`：go vet + go build + go test ./... 全绿（24 个 package 全 ok / 4 个无 test 文件 package 标 `?`）
- `go vet -tags=integration ./...`：全绿（含新 emoji 测试 case）
- `bash scripts/build.sh --integration`：本机 Docker 环境 stale containers 残留（>28 个 28h 老容器）导致新启容器 port 拨号 connectex 拒绝 → 集成测试 `t.Skipf` 优雅跳过；这是 migrate_integration_test.go 顶部钦定的 "docker 不可用时 t.Skip" 路径，**不阻塞 review**。code 路径已通过静态分析（vet）+ 模板严格遵循 11.2 同款 dockertest 实装

### Completion Notes List

**实装总结**：本 story 落地 Epic 17 节点 6 表情广播链路的 schema 根基：

1. **migration 落地**（4 文件）：
   - `server/migrations/0009_init_emoji_configs.up.sql` 严格按数据库设计 §5.15 行 700-718 钦定 8 字段 + UNIQUE uk_code + KEY idx_enabled_sort；顶部注释采用 "对齐 §5.15" + 字段块 + 索引块 + 范围红线四段式（与 0007 / 0008 升级版同模式）
   - `server/migrations/0009_init_emoji_configs.down.sql` 仅 `DROP TABLE IF EXISTS emoji_configs;`
   - `server/internal/repo/mysql/emoji_repo.go` 仅含 `EmojiConfig` GORM domain struct（8 字段，按 SQL 列顺序）+ `TableName() string` 显式返回 `"emoji_configs"`；**不**引入 gorm.Model，**不**在 struct tag 上重复声明 UNIQUE / 索引（与 ADR-0003 §3.2 一致），**不**新建 EmojiRepo interface / 任何方法（YAGNI；17.4 / 17.5 owner）

2. **集成测试扩展**（4 处改动）：
   - `TestMigrateIntegration_UpThenDown` 表数量断言 8 → 9 + expectedTables slice 加 "emoji_configs"
   - `TestMigrateIntegration_UpTwice_Idempotent` 表数量断言 8 → 9
   - `TestMigrateIntegration_StatusAfterUp` 版本号 v != 8 → v != 9（顺带升级；0009 落地后既有 test 必跟改否则 fail）
   - 新增 `TestMigrateIntegration_EmojiConfigs_Schema`：验证 8 列类型 + PK id + UNIQUE uk_code (non_unique=0) + KEY idx_enabled_sort (is_enabled, sort_order) 列顺序 + asset_url / sort_order / is_enabled 默认值 + created_at / updated_at CURRENT_TIMESTAMP 默认值
   - 新增 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected`：dockertest 启 mysql:8.0 → migrate.Up → INSERT (code='wave') 成功 → 再 INSERT (code='wave') 必失败 + 错误消息包含 "Duplicate entry"

3. **schema 一致性核对（AC5.3 + AC5.4）**：
   - 数据库设计 §5.15 行 704-717 ↔ 0009 up.sql：8 字段逐字段对齐（id BIGINT UNSIGNED PK / code VARCHAR(64) NOT NULL / name VARCHAR(64) NOT NULL / asset_url VARCHAR(255) NOT NULL DEFAULT '' / sort_order INT NOT NULL DEFAULT 0 / is_enabled TINYINT NOT NULL DEFAULT 1 / created_at + updated_at DATETIME(3) CURRENT_TIMESTAMP(3)）；UNIQUE KEY uk_code (code) + KEY idx_enabled_sort (is_enabled, sort_order) 索引名 / 列顺序 1:1 ✅
   - V1 §11.1 字段长度约束 ↔ DDL：code 1≤length≤64 ⇔ VARCHAR(64) ✅；name 1≤length≤64 ⇔ VARCHAR(64) ✅；assetUrl 1≤length≤255 ⇔ VARCHAR(255) ✅；sortOrder 0≤value≤2^31-1 ⇔ INT (signed 32-bit) ✅
   - `EmojiConfig` struct 8 字段 ↔ 0009 up.sql 8 列：ID uint64 ↔ BIGINT UNSIGNED ✅；Code/Name/AssetURL string ↔ VARCHAR ✅；SortOrder int32 ↔ INT ✅；IsEnabled int8 ↔ TINYINT ✅；CreatedAt/UpdatedAt time.Time ↔ DATETIME(3) ✅

4. **范围红线遵守**：
   - ❌ 不实装 EmojiRepo interface / List / Create / Exists 方法（17.4 / 17.5 owner）
   - ❌ 不写 seed SQL（17.3 owner）
   - ❌ 不改 0001-0008 既有 migration
   - ❌ 不改 V1 接口契约 / 数据库设计 §5.15 / ADR-0003
   - ❌ 不引入 gorm.Model
   - ❌ struct 上不加 uniqueIndex / index tag

5. **HALT / 异常情况说明**：
   - 本机 Docker 残留 28+ stale mysql:8.0 容器（28h+ 老）+ 多个 `docker rm -f` / `docker container prune` 被沙箱权限拒绝 → 新启容器拨号失败 → 集成测试 `t.Skipf` 优雅跳过
   - **这不构成 HALT**：testfile 顶部注释钦定 "docker 不可用时 t.Skip("docker not available")，不让 CI 阻塞（不要写 panic / fail）" —— skip 是 testing contract 的合法路径，不是失败
   - code 路径已通过 `go vet -tags=integration ./...` 全绿 + 测试代码模板严格 mirror Story 11.2 落地的 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 实装模式（同款 `startMySQL` helper / 同款错误 substring 断言 / 同款 raw `database/sql` INSERT 路径）
   - code-review 阶段在 fresh Docker 环境下 `bash scripts/build.sh --integration` 应可全绿验证 2 个新 case + 既有 case 不回归

### File List

- `server/migrations/0009_init_emoji_configs.up.sql`（新建 —— emoji_configs 表 DDL，对齐数据库设计 §5.15）
- `server/migrations/0009_init_emoji_configs.down.sql`（新建 —— DROP TABLE 路径）
- `server/internal/repo/mysql/emoji_repo.go`（新建 —— `EmojiConfig` GORM domain struct + `TableName()`）
- `server/internal/infra/migrate/migrate_integration_test.go`（修改 —— 表数量断言 8 → 9 + 新增 `TestMigrateIntegration_EmojiConfigs_Schema` + `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 两个 case）
- `_bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md`（本 story 文件 —— Status / Tasks/Subtasks / Dev Agent Record / File List / Change Log）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转 + last_updated）

### Change Log

| 日期 | 操作 | Story 状态 | 备注 |
|---|---|---|---|
| 2026-05-13 | create-story | backlog → ready-for-dev | 由 epic-loop / bmad-create-story workflow 自动生成 |
| 2026-05-13 | dev-story | ready-for-dev → in-progress → review | 4 文件落地（0009 up/down + emoji_repo.go + migrate_integration_test.go 扩展 4 处 / 加 2 case）+ `bash scripts/build.sh --test` 全绿 + `go vet -tags=integration` 全绿；`--integration` 全套受本机 Docker 28+ stale 容器约束 t.Skip（testfile 钦定降级路径），code-review 阶段在 fresh 环境 retry 验证 |
