-- 对齐 docs/宠物互动App_数据库设计.md §5.10 (行 533-567)
-- user_pet_equips 表：宠物穿戴关系表（"哪个宠物的哪个槽位穿了哪一件实例"；
-- 穿戴关系挂到装扮**实例 id**（user_cosmetic_items.id）而非配置 id ——
-- §5.10 设计说明"既然玩家道具已实例化，穿戴关系也应挂到实例 id"）
--
-- **本 migration 由 Story 26.2 首次落地（Epic 26 节点 9 穿戴 / 卸下事务一致性
-- 约束 schema 根基 owner）**
-- 含 ≥3 case 单测（migrate up 后表存在 + 全部约束符合 §5.10，由
--   TestMigrateIntegration_UserPetEquips_Schema 字段层 + 索引层断言覆盖；
--   migrate down 由 TestMigrateIntegration_UpThenDown 扩展覆盖；重复 migrate up
--   幂等由 TestMigrateIntegration_UpTwice_Idempotent 扩展覆盖）
-- + dockertest 集成测试覆盖运行时语义（故意违反两个 UNIQUE 约束 → 数据库拒绝
--   插入，由 TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected
--   覆盖；与 Story 11.2 落地的 TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected
--   同模式 —— 后者验单 + 复合 UNIQUE，本表验两个独立 UNIQUE 各自被拒）。
--
-- **序号纠偏说明**：epics.md §Story 26.2 行 3526 文字写
--   `migrations/0015_init_user_pet_equips.sql`，但 `0015` 已被 Story 23.2 落地的
--   `0015_init_user_cosmetic_items.up/down.sql` 占用（玩家装扮实例表）。
--   epics.md 撰写时点早于 23.2 实际落地 0015，属历史时序错位（与 Story 23.2
--   自身遇到的"epics.md 写 0014 但 0014 被 20.6 占 → 实际用 0015"**同类纠偏
--   先例**）。本 migration 按 ADR-0003 顺序递增编号约定取下一个空闲序号
--   `0016`。这**不**是 §5.10 schema 变更 / 契约变更 / 范围变更（表名
--   user_pet_equips / 字段 / 两个 UNIQUE / 索引钦定全部不变，仅文件序号偏移）。
--
-- 字段（与 §5.10 行 538-550 钦定 1:1 对齐；**全列 NOT NULL，无任何可空列** ——
--   与 Story 23.2 user_cosmetic_items 有 source_ref_id / consumed_at 两个 NULL
--   列不同；本表所有字段 NOT NULL，DDL 不出现任何 NULL 可空声明）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - user_id BIGINT UNSIGNED NOT NULL：归属用户（语义上 ref users.id，
--     **不**建 FK，与本设计其他表一致 —— ADR-0003 / §3 + §7"应用层校验 +
--     索引兜底"策略）
--   - pet_id BIGINT UNSIGNED NOT NULL：归属宠物（语义上 ref pets.id，**不**建 FK）
--   - slot TINYINT NOT NULL：装备槽位（§6.8 枚举：1=hat / 2=gloves /
--     3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other；**无 DEFAULT**
--     —— slot 由 equip 时 cosmetic_items 配置决定必传，非客户端传入，DDL 不做
--     enum 约束、不设 DEFAULT，由 service 层（26.3 按需）+ §6.8 钦定值域兜底）
--   - user_cosmetic_item_id BIGINT UNSIGNED NOT NULL：被穿戴的装扮实例 id
--     （语义上 ref user_cosmetic_items.id，**不**建 FK）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳）
--
-- 关键约束（§5.10 行 547-548 + 行 562-565 设计说明钦定，**两个** UNIQUE）：
--   - UNIQUE KEY uk_pet_slot (pet_id, slot)：一个宠物同一部位只能穿一件
--     （复合 UNIQUE，列顺序 pet_id 先 slot 后）—— Story 26.3 同槽换装 server
--     端自动卸旧装备后 INSERT 新行逻辑的 DB 层兜底 + Story 26.5"并发 1：同 pet
--     同 slot 100 个并发 equip → 只 1 成功"测试的契约依据
--   - UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)：一件实例
--     同时只能装备一次（单列 UNIQUE）—— NFR11 + Story 26.5"并发 2：同实例
--     并发 equip → 只 1 成功"测试的契约依据
--   - KEY idx_user_pet (user_id, pet_id)：普通索引，覆盖 Story 26.6
--     GET /home pet.equips 真实数据"SELECT ... FROM user_pet_equips JOIN
--     cosmetic_items + user_cosmetic_items WHERE user_id=? AND pet_id=?"
--     JOIN 路径，避免 N+1 退化（epics.md §Story 26.6 AC"大量装备并发查 →
--     单 SQL JOIN 不退化"）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （user_pet_equips 是运行时穿戴关系表，**无** seed 阶段；Story 26.3 equip
-- 事务才首条 INSERT）；不含任何业务 service / handler / repo write 或 read
-- 方法（26.3 POST /cosmetics/equip 事务 + 同槽换装 / 26.4 POST /cosmetics/
-- unequip 事务 / 26.6 GET /home pet.equips JOIN 各路径落地）；两个 UNIQUE +
-- 普通索引由本 SQL DDL 定义，GORM struct **不**在 tag 重复声明（ADR-0003
-- §3.2 禁止 GORM AutoMigrate）；不建 FK 约束（与本设计其他表一致）。
CREATE TABLE user_pet_equips (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    pet_id BIGINT UNSIGNED NOT NULL,
    slot TINYINT NOT NULL,
    user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_pet_slot (pet_id, slot),
    UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id),
    KEY idx_user_pet (user_id, pet_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
