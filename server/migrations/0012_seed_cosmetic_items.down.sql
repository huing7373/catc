-- 回滚 0012_seed_cosmetic_items.up.sql
--
-- **本 migration 由 Story 20.3 首次落地（Epic 20 节点 7 宝箱业务链路 seed owner）**
-- 含 ≥4 case 单测 + dockertest 集成测试覆盖 seed 内容正确 + ON DUPLICATE KEY UPDATE
-- 强制覆盖语义。
--
-- 回滚策略：**narrow DELETE 15 行**（精确删 0012 owned 的 15 个 code；
-- **不**用 TRUNCATE 也**不**用全表 DELETE）—— 防误删 admin / dev / 0013+ migration
-- 后续插入的非 0012 seed 数据。
--
-- ============================================================================
-- **最终决策（r0 锁定，沿用 17-3 r3 lesson）**
-- ============================================================================
-- 0012 owns 15 codes（见 0012_seed_cosmetic_items.up.sql 顶部清单）由 0012
-- 完全占用 / 强制覆盖。配套：
--   - **up 强制覆盖** = `INSERT ... ON DUPLICATE KEY UPDATE`（覆盖 7 字段为 0012 钦定值）
--   - **down narrow DELETE** = 当前文件下面这行 SQL（只删 0012 owned 的 15 个 code）
--
-- **强约定（admin / 运维必读）**：
--   1. 0012 owned 的 15 个 code（hat_yellow / hat_red / gloves_white /
--      gloves_brown / glasses_round / neck_blue / back_bag / tail_ribbon /
--      hat_chef / glasses_star / neck_scarf_star / body_tshirt / hat_crown /
--      back_wings / body_armor）**由 0012 完全占用 / 强制覆盖**；admin / 运维
--      **禁止**手工 INSERT / UPDATE 这 15 个 code 的行（up 重跑会被覆盖；
--      down 会被删除；任何 customization 都无法存活）。
--   2. 需要新增装扮（如 season_winter / event_lunar_2026）→ 通过**新 migration**
--      （0013+）添加，不要在 cosmetic_items 表上做 admin 直插。
--   3. 本 down 是 **narrow DELETE 15 行**（不是 TRUNCATE、不是全表 DELETE）：
--      只删 15 个钦定 code，**不会动** 0013+ 加的新装扮 / admin 手工 INSERT 的
--      非 owned codes（如 admin 测试时插的 `code='admin_test_001'` 行）。
--
-- 详见 lesson：
--   - docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md (17-3 r2)
--   - docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md (17-3 r3, **最终决断**)
--
-- **down 实际执行场景**（与 golang-migrate 语义对齐）：
--   (a) 单跑 0012.down（保留 0011 schema）→ DELETE 15 行 → schema_migrations 回退到 v=11
--       → cosmetic_items 表存在但 0012 owned 15 个 code 不在；再 up 一次会重新跑
--       0012.up（ON DUPLICATE KEY UPDATE）把 15 行 INSERT/覆盖回来。
--   (b) 跑 0011.down 链式带 0012.down 先跑 → 先 DELETE 15 行 → 再 DROP TABLE
--       cosmetic_items → 整张表清理；语义自洽。
DELETE FROM cosmetic_items WHERE code IN (
    'hat_yellow', 'hat_red', 'gloves_white', 'gloves_brown',
    'glasses_round', 'neck_blue', 'back_bag', 'tail_ribbon',
    'hat_chef', 'glasses_star', 'neck_scarf_star', 'body_tshirt',
    'hat_crown', 'back_wings', 'body_armor'
);
