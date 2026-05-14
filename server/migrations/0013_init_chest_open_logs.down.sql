-- 回滚 0013_init_chest_open_logs.up.sql
--
-- **本 migration 由 Story 20.4 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖 append-only 语义（同 user_id 多行允许）。
DROP TABLE IF EXISTS chest_open_logs;
