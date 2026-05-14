-- 对齐 docs/宠物互动App_数据库设计.md §5.8 (行 437-479)
-- cosmetic_items 表：装扮配置表（"装扮是什么"，不是"玩家拥有哪一件"）
--
-- **本 migration 由 Story 20.2 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 + 字段类型 + uk_code + idx_slot_rarity + idx_enabled_weight 索引符合 §5.8）
-- + dockertest 集成测试覆盖 UNIQUE KEY uk_code (code) 运行时 INSERT 拒绝行为
-- （epics.md §Story 20.2 钦定的"集成测试覆盖：migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径
-- 升级为更精确的 INFORMATION_SCHEMA 字段层断言 + 额外 UNIQUE 拒绝运行时验证）。
--
-- 字段（与 §5.8 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - code VARCHAR(64) NOT NULL：装扮业务编码（如 hat_yellow / scarf_star）；
--     与 V1 接口设计 §7.2 / §8.1 / §8.2 锚定的 cosmeticItem code 字段同语义；
--     UNIQUE KEY uk_code 保证全局唯一（Story 20.3 seed 用 INSERT IGNORE / ON DUPLICATE KEY UPDATE
--     兜底重复执行）
--   - name VARCHAR(64) NOT NULL：装扮中文名（如 小黄帽 / 星星围巾），
--     client 用作 UI 展示文字；与 V1 §7.2 reward.name 字段同源（1 ≤ length ≤ 64）
--   - slot TINYINT NOT NULL：部位枚举（§6.8 钦定：1=hat / 2=gloves / 3=glasses /
--     4=neck / 5=back / 6=body / 7=tail / 99=other）；
--     **注**：DDL 不在 schema 层做 enum 约束（TINYINT 允许任意 -128~127 值），
--     由 server seed 层（20.3）+ admin 写入层校验 enum 值合法性；
--     与 V1 §7.2 reward.slot 字段同源（int 枚举）
--   - rarity TINYINT NOT NULL：品质枚举（§6.9 钦定：1=common / 2=rare /
--     3=epic / 4=legendary）；
--     同 slot：DDL 不在 schema 层做 enum 约束，由 service 层 + seed 校验；
--     与 V1 §7.2 reward.rarity 字段同源（int 枚举）；
--     与 chest_open_logs.reward_rarity 字段（§5.7 行 411）同语义（开箱日志同步记录）
--   - asset_url VARCHAR(255) NOT NULL DEFAULT ''：装扮资源 URL；
--     **注**：DDL DEFAULT '' 是兜底语义（避免 admin / dev 临时写入时漏字段），
--     enabled 装扮（is_enabled=1）必须有非空 asset_url —— Story 20.3 seed 钦定
--     每个 cosmetic 非空（按 AR18 / AR19 URL 约束，MVP 允许 placeholder URL
--     如 https://placehold.co/128x128?text=Hat-Yellow）；
--     与 V1 §7.2 reward.assetUrl 字段同源（1 ≤ length ≤ 255）
--   - icon_url VARCHAR(255) NOT NULL DEFAULT ''：图标资源 URL（仓库 / catalog 列表用小图）；
--     同 asset_url：enabled 装扮必须非空；
--     与 V1 §7.2 reward.iconUrl 字段同源（1 ≤ length ≤ 255）
--   - drop_weight INT UNSIGNED NOT NULL DEFAULT 0：掉落权重（用于 Story 20.6
--     加权抽奖；按品质递减如 common=100 / rare=20 / epic=4 / legendary=1，
--     由 Story 20.3 seed 钦定，本 migration 仅定义字段不预置数据）；
--     INT UNSIGNED 是 32 位无符号（0 ~ 2^32 - 1），抽奖时权重为 0 的行被
--     `WHERE drop_weight > 0` 过滤掉
--   - is_enabled TINYINT NOT NULL DEFAULT 1：是否启用（§6 枚举：0=disabled / 1=enabled）；
--     §8.1 GET /cosmetics/catalog（Epic 23）仅返回 is_enabled=1 的 cosmetic；
--     §7.2 POST /chest/open 加权抽取仅命中 is_enabled=1 的行（Story 20.6 实装 SQL
--     `WHERE is_enabled = 1` 走 idx_enabled_weight 索引覆盖）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳）
--
-- 索引（§5.8 + §7 钦定）：
--   - UNIQUE KEY uk_code (code)：保证 code 全局唯一（§7.1 高优先级 UNIQUE 约束，
--     与 emoji_configs.uk_code 同模式）；
--     Story 20.3 seed 用 INSERT IGNORE / ON DUPLICATE KEY UPDATE 兜底重复执行
--   - KEY idx_slot_rarity (slot, rarity)：覆盖
--     "SELECT ... WHERE slot=? AND rarity=?" 路径（Epic 32 合成事务需要按
--     slot + rarity 维度做加权抽取，本索引提前覆盖）；
--     节点 7 阶段 Story 20.6 加权抽取暂不按 slot/rarity 维度（按全局 drop_weight），
--     本索引是节点 11 合成事务的提前准备（与 §5.8 钦定一致，不延后到 Epic 32）
--   - KEY idx_enabled_weight (is_enabled, drop_weight)：覆盖
--     "SELECT ... WHERE is_enabled=1 AND drop_weight > 0 ORDER BY ..." 路径
--     （Story 20.6 加权抽取 SQL；多列复合索引覆盖筛选 + 权重排序）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （seed 由 Story 20.3 `0012_seed_cosmetic_items.up.sql` 落地，**不**塞进本文件）；
-- 不含任何业务 service / handler / repo write 方法（20.3 ~ 20.9 落地）；
-- 不含 render_config 字段（节点 10 / Epic 29 Story 29.2 落地 `001X_add_render_config_to_cosmetic_items.up.sql`
-- 新 migration 加列，**不**在本 story 范围）。
CREATE TABLE cosmetic_items (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    slot TINYINT NOT NULL,
    rarity TINYINT NOT NULL,
    asset_url VARCHAR(255) NOT NULL DEFAULT '',
    icon_url VARCHAR(255) NOT NULL DEFAULT '',
    drop_weight INT UNSIGNED NOT NULL DEFAULT 0,
    is_enabled TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_code (code),
    KEY idx_slot_rarity (slot, rarity),
    KEY idx_enabled_weight (is_enabled, drop_weight)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
