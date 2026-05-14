-- 回滚 0011_init_cosmetic_items.up.sql
--
-- **本 migration 由 Story 20.2 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为。
DROP TABLE IF EXISTS cosmetic_items;
