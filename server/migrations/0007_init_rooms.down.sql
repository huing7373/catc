-- 回滚 0007_init_rooms.up.sql
--
-- **本 migration 由 Story 10.3 review r5 [P1] 引入；Story 11.2 正式接管 Epic 11 owner**
-- （含完整 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为 +
-- WS 集成测试 fixture 切到 official migration 路径）。
-- DDL（DROP TABLE 顺序）严格不动；注释升级仅为 audit trail / Epic 11 retrospective
-- owner 追溯使用。
DROP TABLE IF EXISTS rooms;
