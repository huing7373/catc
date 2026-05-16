-- 回滚 0015_init_user_cosmetic_items.up.sql
--
-- **本 migration 由 Story 23.2 首次落地（Epic 23 节点 8 仓库 / 穿戴 / 合成业务链路
-- schema 根基 owner；序号 0015 纠偏说明见 .up.sql 注释头）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖运行时语义（同 user_id 多行 INSERT 无
-- UNIQUE 拒绝 + status / consumed_at 可 UPDATE 推进 + source_ref_id NULL 默认）。
DROP TABLE IF EXISTS user_cosmetic_items;
