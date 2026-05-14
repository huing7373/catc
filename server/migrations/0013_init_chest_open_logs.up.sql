-- 对齐 docs/宠物互动App_数据库设计.md §5.7 (行 399-433)
-- chest_open_logs 表：开箱日志表（append-only，**无** updated_at；
-- 单独保存，方便追踪掉落问题 + 用户历史展示 + 运营分析与概率排查）
--
-- **本 migration 由 Story 20.4 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 + 字段类型 + idx_user_id_created_at + idx_reward_cosmetic_item_id 索引符合 §5.7）
-- + dockertest 集成测试覆盖 append-only 语义（同 user_id 可插入多行，无 UNIQUE 拒绝；
--   与 20.2 落地的 TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected 形成对照
--   —— 后者验证有 UNIQUE 的运行时拒绝，本表验证无 UNIQUE 的多行允许）。
--
-- 字段（与 §5.7 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - user_id BIGINT UNSIGNED NOT NULL：归属用户（语义上 reference users.id，
--     **不**建 FK，与本设计其他表保持一致 —— ADR-0003 / 数据库设计 §3 + §7 钦定的
--     "应用层校验 + 索引兜底"策略）
--   - chest_id BIGINT UNSIGNED NOT NULL：被开启的宝箱 id（语义上 reference
--     user_chests.id；同样**不**建 FK；§5.7 字段说明行 421 钦定）
--   - cost_steps INT UNSIGNED NOT NULL：实际消耗步数（§5.7 字段说明行 422 钦定；
--     节点 7 阶段 Story 20.6 钦定 = 1000，V1 §7.2 cost.amount 字段同源；INT UNSIGNED
--     范围 0 ~ 2^32 - 1，足够覆盖任意单次开箱步数）
--   - reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL：产出的装扮实例 id
--     （§5.7 字段说明行 423 钦定；**关键节点 7 阶段语义**：固定为 `0` 占位，因不
--     创建 user_cosmetic_items 实例 —— V1接口设计 §7.2.4h + 数据库设计 §8 注解钦定
--     "节点 7 阶段 reward_user_cosmetic_item_id 写占位 0"；节点 8 Epic 23 Story 23.5
--     修改开箱事务为先 INSERT user_cosmetic_items 拿到 id 再填入此处，本 migration
--     DDL `BIGINT UNSIGNED NOT NULL` 已为该切换预留语义 —— unsigned 范围支持任意
--     非负 id；NOT NULL 兜底防漏写；DEFAULT 不设值由 service 层显式提供 0 / 真实 id）
--   - reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL：产出的装扮配置 id
--     （§5.7 字段说明行 424 钦定；语义上 reference cosmetic_items.id —— 20.2 落地
--     的表；**不**建 FK；Story 20.6 加权抽取 SQL 命中后从抽到的 cosmetic_items.id
--     写入此字段）
--   - reward_rarity TINYINT NOT NULL：奖励品质（§5.7 字段说明行 425 钦定；§6.9
--     枚举 1=common / 2=rare / 3=epic / 4=legendary；Story 20.6 加权抽取后写入
--     cosmetic_items.rarity 同步到此处，与 V1 §7.2 reward.rarity 字段同源；
--     **注**：DDL 不在 schema 层做 enum 约束（TINYINT 允许任意 -128~127 值），
--     由 service 层 + cosmetic_items 表的合法 rarity 兜底）
--   - created_at DATETIME(3)（§3.2 毫秒精度时间戳；append-only 日志表**无**
--     updated_at —— 与 0006 user_step_sync_logs 同模式）
--
-- 索引（§5.7 + §7.2 高优先级普通索引清单 行 937 钦定）：
--   - KEY idx_user_id_created_at (user_id, created_at)：覆盖
--     "SELECT ... WHERE user_id=? ORDER BY created_at DESC LIMIT N" 路径
--     （未来运营 / 用户历史展示按用户按时间倒序查最近 N 次开箱；
--     §7.2 高优先级普通索引清单 chest_open_logs(user_id, created_at) 钦定）
--   - KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)：覆盖
--     "SELECT ... WHERE reward_cosmetic_item_id=?" 路径
--     （运营分析 / 概率排查按 cosmetic 倒查产出分布；§5.7 设计说明行 433 钦定
--     "便于运营分析与概率排查"）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （chest_open_logs 是运行时日志表，**无** seed 阶段；Story 20.6 开箱事务才首条
-- INSERT）；不含任何业务 service / handler / repo write 方法（20.6 ~ 20.9 落地）；
-- 不含 updated_at 字段（append-only 日志表语义，与 0006 user_step_sync_logs 同模式）；
-- 不含 UNIQUE 约束（日志表允许同 user_id 多次开箱）；不建 FK 约束（与本设计其他表
-- 保持一致）。
CREATE TABLE chest_open_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    chest_id BIGINT UNSIGNED NOT NULL,
    cost_steps INT UNSIGNED NOT NULL,
    reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_rarity TINYINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_id_created_at (user_id, created_at),
    KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
