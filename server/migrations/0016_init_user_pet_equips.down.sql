-- 回滚 0016_init_user_pet_equips.up.sql
--
-- **本 migration 由 Story 26.2 首次落地（Epic 26 节点 9 穿戴 / 卸下事务一致性
-- 约束 schema 根基 owner；序号 0016 纠偏说明见 .up.sql 注释头 —— 0015 被
-- Story 23.2 user_cosmetic_items 占用）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖运行时语义（故意违反两个 UNIQUE
-- 约束 uk_pet_slot / uk_user_cosmetic_item_id → 数据库拒绝插入）。
DROP TABLE IF EXISTS user_pet_equips;
