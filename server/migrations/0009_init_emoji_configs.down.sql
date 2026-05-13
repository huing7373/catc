-- 回滚 0009_init_emoji_configs.up.sql
--
-- **本 migration 由 Story 17.2 首次落地（Epic 17 节点 6 表情广播链路 owner）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为。
DROP TABLE IF EXISTS emoji_configs;
