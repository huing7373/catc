-- 对齐 epics.md §Story 17.3 + AR19 + V1接口设计.md §11.1
-- emoji_configs 系统表情配置 seed
--
-- **本 migration 由 Story 17.3 首次落地（Epic 17 节点 6 表情广播链路 seed owner）**
-- 含 ≥2 case 单测（seed 后 ≥4 行 + asset_url 都非空 / 重复 migrate up 不重复插入）
-- + dockertest 集成测试覆盖 seed 内容正确 + ON DUPLICATE KEY UPDATE 强制覆盖
-- （epics.md §Story 17.3 钦定的"集成测试覆盖：migrate up → SELECT * FROM emoji_configs
-- → 验证 4 个表情存在 + URL 字段格式合法"路径）。
--
-- 表情清单（与 epics.md §Story 17.3 行 2569-2573 钦定 1:1 对齐）：
--   1. wave  挥手     sort_order=1
--   2. love  爱心     sort_order=2
--   3. laugh 大笑     sort_order=3
--   4. cry   哭       sort_order=4
--
-- 字段值约束：
--   - code:       严格符合 V1 §11.1 字符集约束 [a-z0-9_-] + length 1-64
--                 （本 4 个 code 都是纯小写英文字母，长度 3-5，合法）
--   - name:       中文短名，长度 ≤ 64（VARCHAR(64) DDL 边界）
--   - asset_url:  非空 placeholder URL（V1 §11.1 + 17-1 r2 lesson 钦定：
--                 enabled 表情 asset_url **禁止**空字符串；MVP 阶段允许 placeholder
--                 URL `https://placehold.co/64x64?text=Wave` 等，但**必须**是可
--                 访问的 web URL；真实美术资产由 §Epic 17 retrospective tech-debt
--                 登记 + 后续 epic 切换）
--   - sort_order: 1 / 2 / 3 / 4（单调递增 + 唯一；与 V1 §11.1 服务端逻辑步骤 2
--                 `ORDER BY sort_order ASC, id ASC` 一致；4 个值互不相同保证 client
--                 端表情面板顺序稳定，不需要次要排序键 fallback）
--   - is_enabled: 全部 1（enabled；V1 §11.1 服务端逻辑步骤 2 仅返回 is_enabled=1
--                 的表情，disabled 表情对 client 不可见）
--
-- ============================================================================
-- **最终决策（17-3 r3 锁定，经过 r1 / r2 / r3 三轮 codex review 反复打架后定稿）**
-- ============================================================================
-- **wave / love / laugh / cry 这 4 个 code 由 0010 完全占用 / 强制覆盖**。
-- admin / 运维 **禁止**在这 4 个 code 上做 customization；如要加 emoji **必须**新建
-- migration 0011+（如 `0011_seed_emoji_angry.up.sql`）。
--
-- 配套：
--   - **up 强制覆盖** = `INSERT ... ON DUPLICATE KEY UPDATE`（覆盖 name / asset_url
--     / sort_order / is_enabled 4 字段为 0010 钦定值）
--   - **down narrow DELETE** = `DELETE FROM emoji_configs WHERE code IN (wave/love/laugh/cry)`
--
-- 这一对决策共同保证：Story 17.4/17.5/18.1 依赖的 **4 个 enabled emoji 配置 invariant
-- 100% 强保证**，不依赖任何 admin 自律。即使 admin 误操作 / dev 历史残留 /
-- migrate force / 手工 mysql import 等异常路径让 emoji_configs 表里预先存在
-- 这 4 个 code 的"坏行"（is_enabled=0 / asset_url='' / sort_order 乱序），
-- 跑 0010.up 后**也会**被覆盖回 0010 钦定值。
--
-- **三轮 review 演化简史**（关键 commit 不可丢失的上下文）：
--   - r1 [P2]: down DELETE 删 admin 数据 → 改 0010.down no-op `SELECT 1;`
--   - r2 [P2]: no-op down 违反 golang-migrate invariant "down 必须真正 undo up"
--     → 改回 narrow DELETE 4 行；admin 数据保留通过"约定 + 新 migration"兜底
--   - r3 [P1 #1]: r2 决策下 up `INSERT IGNORE` 仍让预存的坏行（is_enabled=0 等）
--     幸存 → Story 17.4/17.5/18.1 依赖的 invariant 无法保证
--     → 改 up 为 `INSERT ... ON DUPLICATE KEY UPDATE` 强制覆盖 4 字段；同时
--     r3 [P1 #2] 重提 r1 "down 不应删 admin 数据"被 wontfix（与 r2 决策冲突，
--     且 up 强制覆盖后 admin 在这 4 个 code 上的 customization 无法在 up 时存活；
--     down DELETE 是 up 强制覆盖语义的对称延续）
--
-- 详见 lesson：
--   - docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md (r2)
--   - docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md (r3, **最终决断**)
--
-- **范围红线**：本 migration **仅** UPSERT 4 行；不修改 schema（17.2 owner）/
-- 不含任何业务 service / handler / repo write 方法（17.4 / 17.5 落地）/ 不含
-- TRUNCATE 任何破坏性 SQL。
--
-- **不**用 server 端代码层（如 Go 的 seedEmojis() 函数）做 seed：
-- (a) ADR-0003 钦定 migrations/ 文件是 schema + 静态数据的真相源；
-- (b) seed 通过 SQL migration 让 dev / test / staging / prod 同一份数据；
-- (c) 与 cosmetic seed（Epic 20）未来落地路径同模式，避免每个 seed 自己定一种执行方式。
INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES
    ('wave',  '挥手', 'https://placehold.co/64x64?text=Wave',  1, 1),
    ('love',  '爱心', 'https://placehold.co/64x64?text=Love',  2, 1),
    ('laugh', '大笑', 'https://placehold.co/64x64?text=Laugh', 3, 1),
    ('cry',   '哭',   'https://placehold.co/64x64?text=Cry',   4, 1)
ON DUPLICATE KEY UPDATE
    name       = VALUES(name),
    asset_url  = VALUES(asset_url),
    sort_order = VALUES(sort_order),
    is_enabled = VALUES(is_enabled);
