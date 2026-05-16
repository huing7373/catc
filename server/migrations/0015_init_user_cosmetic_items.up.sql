-- 对齐 docs/宠物互动App_数据库设计.md §5.9 (行 483-523)
-- user_cosmetic_items 表：玩家装扮实例表（"玩家拥有哪一件"，每件道具一个唯一 id；
-- 不是 cosmetic_items 配置表的"装扮是什么"——见 §5.8 / 0011_init_cosmetic_items）
--
-- **本 migration 由 Story 23.2 首次落地（Epic 23 节点 8 仓库 / 穿戴 / 合成业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 + 字段类型 + 三个普通索引符合 §5.9，由
--   TestMigrateIntegration_UserCosmeticItems_Schema 字段层断言覆盖；migrate down
--   由 TestMigrateIntegration_UpThenDown 扩展覆盖；重复 migrate up 幂等由
--   TestMigrateIntegration_UpTwice_Idempotent 扩展覆盖）
-- + dockertest 集成测试覆盖运行时语义（同 user_id 多行 INSERT 无 UNIQUE 拒绝 +
--   status / consumed_at 可 UPDATE 推进 + source_ref_id NULL 默认，由
--   TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable 覆盖；
--   与 Story 20.4 落地的 TestMigrateIntegration_ChestOpenLogs_AppendOnly 形成对照
--   —— 后者验证 append-only 不可 UPDATE，本表是可变实例表 status 可推进）。
--
-- **序号纠偏说明**：epics.md §Story 23.2 文字写 `migrations/0014_init_user_cosmetic_items.sql`，
--   但 `0014` 已被 Story 20.6 落地的 `0014_init_chest_open_idempotency_records.up/down.sql`
--   占用（开箱接口幂等记录表）。epics.md 撰写时点早于 20.6 实际落地 0014，属历史
--   时序错位；本 migration 按 ADR-0003 顺序递增编号约定取下一个空闲序号 `0015`。
--   这**不**是 §5.9 schema 变更 / 契约变更 / 范围变更（表名 / 字段 / 索引钦定全部不变）。
--
-- 字段（与 §5.9 行 487-503 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定；§5.9 字段说明
--     行 508 钦定"玩家道具实例 id，即每一个道具的唯一 id"）
--   - user_id BIGINT UNSIGNED NOT NULL：归属用户（语义上 reference users.id，
--     **不**建 FK，与本设计其他表保持一致 —— ADR-0003 / 数据库设计 §3 + §7 钦定的
--     "应用层校验 + 索引兜底"策略）
--   - cosmetic_item_id BIGINT UNSIGNED NOT NULL：对应的装扮配置 id（§5.9 字段说明
--     行 509；语义上 reference cosmetic_items.id —— 20.2 落地的表；同样**不**建 FK）
--   - status TINYINT NOT NULL DEFAULT 1：当前实例状态（§6.10 枚举：1=in_bag /
--     2=equipped / 3=consumed / 4=invalid）；DEFAULT 1 = 新建实例默认 in_bag；
--     **注**：DDL 不在 schema 层做 enum 约束（TINYINT 允许任意 -128~127 值），
--     由 service 层（23.4 / 23.5 / Epic 26 / 32 实装时按需）+ §6.10 钦定值域兜底
--   - source TINYINT NOT NULL DEFAULT 1：来源类型（§6.11 枚举：1=chest /
--     2=compose / 3=admin_grant / 4=event_reward）；DEFAULT 1 = 默认开箱来源；
--     同 status：DDL 不做 enum 约束，由 service 层 + §6.11 钦定值域兜底
--   - source_ref_id BIGINT UNSIGNED NULL：来源关联记录 id（§5.9 字段说明行 512）；
--     **可空** —— 开箱来源 source_ref_id=chest_id 非空，但合成产出实例时先 NULL
--     后回填 compose_log_id，故 DDL 必须 NULL（GORM struct 用 *uint64 指针映射，
--     NULL 与 0 语义区分）
--   - obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)：获得时间
--     （§5.9 字段说明行 513；§3.2 毫秒精度时间戳）
--   - consumed_at DATETIME(3) NULL：消耗时间（§5.9 字段说明行 514 钦定"未消耗时
--     为空"）；**可空** —— 未消耗时为 NULL，合成消耗时写 NOW(3)（GORM struct 用
--     *time.Time 指针映射）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳；updated_at 在
--     status / consumed_at 推进时自动更新 —— 实例可变表，与 0013 append-only
--     日志表无 updated_at 形成对照）
--
-- 索引（§5.9 行 500-502 + §7 钦定，全部普通索引，**无** UNIQUE）：
--   - KEY idx_user_id_status (user_id, status)：覆盖 §8.2 GET /cosmetics/inventory
--     "SELECT id, cosmetic_item_id, status FROM user_cosmetic_items
--      WHERE user_id=? AND status IN (1,2)" 路径（23.4 实装；Epic 26 穿戴时
--     status 在 1↔2 推进也走此索引）
--   - KEY idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id)：覆盖按
--     cosmetic_item_id 维度聚合（23.4 inventory grid 按配置分组）+ Epic 26 穿戴
--     时按 (user_id, cosmetic_item_id) 查实例路径
--   - KEY idx_source (source, source_ref_id)：覆盖运营按来源倒查产出分布路径
--     （source=1 倒查某 chest 产出 / source=2 倒查某 compose_log 产出）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （user_cosmetic_items 是运行时实例表，**无** seed 阶段；Story 23.5 开箱事务才
-- 首条 INSERT）；不含任何业务 service / handler / repo write 或 read 方法
-- （23.4 GET /cosmetics/inventory / 23.5 开箱补入仓 / Epic 26 穿戴 / Epic 32
-- 合成各路径落地）；不含 UNIQUE 约束（实例表，同 user_id + 同 cosmetic_item_id
-- 可持有多件 —— FR16"同种配置可被持有多件"；与 0013 chest_open_logs 无 UNIQUE
-- 同模式，但本表实例 status 可被 UPDATE 推进，**非** append-only）；不建 FK
-- 约束（与本设计其他表一致）。
CREATE TABLE user_cosmetic_items (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL DEFAULT 1,
    source TINYINT NOT NULL DEFAULT 1,
    source_ref_id BIGINT UNSIGNED NULL,
    obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    consumed_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    KEY idx_user_id_status (user_id, status),
    KEY idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id),
    KEY idx_source (source, source_ref_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
