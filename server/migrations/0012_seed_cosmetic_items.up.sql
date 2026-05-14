-- 对齐 epics.md §Story 20.3 + AR18 + V1接口设计.md §7.2 + 数据库设计.md §5.8 / §6.8 / §6.9
-- cosmetic_items 装扮配置 seed
--
-- **本 migration 由 Story 20.3 首次落地（Epic 20 节点 7 宝箱业务链路 seed owner）**
-- 含 ≥4 case 单测（seed 后 ≥15 行 + AR18 各品质数量 + common 至少覆盖 4 个不同 slot +
-- 每行 URL 非空 + 重复 migrate up 不重复插入 + ON DUPLICATE KEY UPDATE 强制覆盖）
-- + dockertest 集成测试覆盖 seed 内容正确 + 强制覆盖语义。
--
-- 装扮清单（满足 AR18 数量约束，与 epics.md §Story 20.3 行 2838-2845 钦定一致；
-- 总 15 件：common 8 / rare 4 / epic 2 / legendary 1）：
--
--   common（8 件，至少覆盖 4 个不同 slot；drop_weight=100）：
--     1.  hat_yellow      小黄帽       slot=1(hat)     rarity=1(common)
--     2.  hat_red         小红帽       slot=1(hat)     rarity=1(common)
--     3.  gloves_white    白手套       slot=2(gloves)  rarity=1(common)
--     4.  gloves_brown    棕手套       slot=2(gloves)  rarity=1(common)
--     5.  glasses_round   圆框眼镜     slot=3(glasses) rarity=1(common)
--     6.  neck_blue       蓝围脖       slot=4(neck)    rarity=1(common)
--     7.  back_bag        小书包       slot=5(back)    rarity=1(common)
--     8.  tail_ribbon     蝴蝶结尾巾   slot=7(tail)    rarity=1(common)
--   → common 覆盖 slot ∈ {1, 2, 3, 4, 5, 7}，共 6 个不同槽位（≥ 4 满足 AR18）
--
--   rare（4 件；drop_weight=20）：
--     9.  hat_chef        厨师帽       slot=1(hat)     rarity=2(rare)
--     10. glasses_star    星星眼镜     slot=3(glasses) rarity=2(rare)
--     11. neck_scarf_star 星星围巾     slot=4(neck)    rarity=2(rare)
--     12. body_tshirt     白T恤        slot=6(body)    rarity=2(rare)
--
--   epic（2 件；drop_weight=4）：
--     13. hat_crown       金王冠       slot=1(hat)     rarity=3(epic)
--     14. back_wings      天使翅膀     slot=5(back)    rarity=3(epic)
--
--   legendary（1 件；drop_weight=1）：
--     15. body_armor      黄金圣衣     slot=6(body)    rarity=4(legendary)
--
-- 字段值约束：
--   - code:       严格符合 §5.8 VARCHAR(64) + 业务命名约定 `{slot}_{name_en}`
--                 （如 hat_yellow / gloves_white / glasses_star）；
--                 与未来 Epic 29 Story 29.3 render_config seed 的 UPDATE 字面量 1:1 对齐
--   - name:       中文短名，长度 ≤ 64（§5.8 VARCHAR(64) NOT NULL）；
--                 与 V1 §7.2 reward.name `1 ≤ length ≤ 64` 一致
--   - slot:       §6.8 枚举值 ∈ {1,2,3,4,5,6,7,99}（hat / gloves / glasses /
--                 neck / back / body / tail / other）；本 seed **不**用 99=other
--                 槽位（保留给特殊冠名 cosmetic）
--   - rarity:     §6.9 枚举值 ∈ {1,2,3,4}（common / rare / epic / legendary）
--   - asset_url:  非空 placeholder URL `https://placehold.co/128x128?text={EnglishName}`
--                 （V1 §7.2 reward.assetUrl 1 ≤ length ≤ 255 + 禁止空字符串；AR18
--                 钦定 MVP 阶段允许 placeholder URL，真实美术资产由 Epic 20 retro
--                 / 后续 epic 切换；与 17.3 placehold.co 同源服务）
--   - icon_url:   非空 placeholder URL `https://placehold.co/64x64?text={EnglishName}`
--                 （V1 §7.2 reward.iconUrl 1 ≤ length ≤ 255 + 禁止空字符串；
--                 64x64 是小尺寸预览图，仓库 grid / 奖励弹窗 icon 用）
--   - drop_weight: 按品质递减分布（common=100 / rare=20 / epic=4 / legendary=1，
--                 与 epics.md §Story 20.3 行 2844 钦定一致）；权重比 100:20:4:1 ≈
--                 25:5:1:0.25 让连开 20 次能命中 3-4 种不同 cosmetic（验证 AR18
--                 + Story 19.1 节点 7 demo E2E "验证场景 8 抽奖多样性"）
--   - is_enabled: 全部 1（enabled；V1 §7.2 加权抽取 + §8.1 GET /cosmetics/catalog
--                 仅返回 / 命中 is_enabled=1 的 cosmetic）
--
-- ============================================================================
-- **最终决策（r0 锁定，沿用 17-3 r3 lesson；不再走 r1 / r2 / r3 三轮 review）**
-- ============================================================================
-- **0012 owns 15 codes**（hat_yellow / hat_red / gloves_white / gloves_brown /
-- glasses_round / neck_blue / back_bag / tail_ribbon / hat_chef / glasses_star /
-- neck_scarf_star / body_tshirt / hat_crown / back_wings / body_armor）由 0012
-- 完全占用 / 强制覆盖。admin / 运维 **禁止**在这 15 个 code 上做 customization；
-- 如要加 cosmetic **必须**新建 migration 0013+（如 `0013_seed_cosmetic_seasonal.up.sql`）。
--
-- 配套：
--   - **up 强制覆盖** = `INSERT ... ON DUPLICATE KEY UPDATE`（覆盖 name / slot / rarity /
--     asset_url / icon_url / drop_weight / is_enabled 7 字段为 0012 钦定值）
--   - **down narrow DELETE** = `DELETE FROM cosmetic_items WHERE code IN (15 个钦定 code)`
--
-- 这一对决策共同保证：Story 20.5 / 20.6 / 20.8 / Epic 21 / Epic 23 / Epic 29 Story 29.3 /
-- Epic 32 / 33 依赖的 **15 个 enabled cosmetic 配置 invariant 100% 强保证**，
-- 不依赖任何 admin 自律。即使 admin 误操作 / dev 历史残留 / migrate force / 手工
-- mysql import 等异常路径让 cosmetic_items 表里预先存在这 15 个 code 的"坏行"
-- （is_enabled=0 / asset_url='' / drop_weight 漂移 / rarity 漂移 / slot 漂移 /
-- icon_url=''），跑 0012.up 后**也会**被覆盖回 0012 钦定值。
--
-- **r0 直接复用 17-3 r3 最终决断**（不再走 r1 / r2 / r3 三轮反复）：
--   - 17.3 落地 emoji_configs seed 时经历 INSERT IGNORE → no-op down → narrow DELETE
--     down → ON DUPLICATE KEY UPDATE up 的反复决策路径，最终落定为本 story 直接
--     沿用的"up 强制覆盖 + down narrow DELETE"模式
--   - 详见 lesson：
--     - docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md (17-3 r2)
--     - docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md (17-3 r3, **最终决断**)
--
-- **不**用 INSERT IGNORE：因为 IGNORE 路径下，admin 预存的"坏行"（如 `is_enabled=0`
-- 把某 cosmetic 临时下架 / `asset_url=''` 让奖励弹窗破图 / `drop_weight=0` 让某
-- cosmetic 永远抽不到）会**幸存** → Story 20.6 加权抽取 + 21.4 奖励弹窗 + 19.1
-- E2E 钦定的 "AR18 数量约束生效"无法 100% 保证。ON DUPLICATE KEY UPDATE 强制覆盖
-- 是唯一能让"15 个 enabled cosmetic invariant"100% 强保证的路径。
--
-- **不**用 REPLACE INTO：REPLACE 是 DELETE+INSERT 实现，会重新分配 id；
-- 虽然 0011 schema 当前 cosmetic_items.id 没被外键引用（user_cosmetic_items /
-- chest_open_logs.reward_cosmetic_item_id 都不建 FK），但 REPLACE 触发的 id 变化
-- 会让 chest_open_logs.reward_cosmeticItemId 的历史日志和当前 cosmetic_items.id
-- 断开 reference 语义；ON DUPLICATE KEY UPDATE 走 UPDATE 路径保留 id 不变，安全。
--
-- **范围红线**：本 migration **仅** UPSERT 15 行；不修改 schema（20.2 owner）/
-- 不含任何业务 service / handler / repo write 方法（20.5 / 20.6 / 20.7 / 20.8 /
-- 23.3 落地）/ 不含 TRUNCATE / 全表 DELETE 任何破坏性 SQL / 不含 render_config
-- 字段（节点 10 / Epic 29 Story 29.2 加列 + 29.3 seed owner，本 story 故意保留扩展空间）。
--
-- **不**用 server 端代码层（如 Go 的 seedCosmetics() 函数）做 seed：
-- (a) ADR-0003 钦定 migrations/ 文件是 schema + 静态数据的真相源；
-- (b) seed 通过 SQL migration 让 dev / test / staging / prod 同一份数据；
-- (c) 与 emoji seed（0010_seed_emoji_configs）落地路径同模式，避免每个 seed 自己
--     定一种执行方式。
INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES
    -- common（8 件，覆盖 6 个不同 slot；drop_weight=100）
    ('hat_yellow',      '小黄帽',       1, 1, 'https://placehold.co/128x128?text=Hat-Yellow',     'https://placehold.co/64x64?text=Hat-Yellow',     100, 1),
    ('hat_red',         '小红帽',       1, 1, 'https://placehold.co/128x128?text=Hat-Red',        'https://placehold.co/64x64?text=Hat-Red',        100, 1),
    ('gloves_white',    '白手套',       2, 1, 'https://placehold.co/128x128?text=Gloves-White',   'https://placehold.co/64x64?text=Gloves-White',   100, 1),
    ('gloves_brown',    '棕手套',       2, 1, 'https://placehold.co/128x128?text=Gloves-Brown',   'https://placehold.co/64x64?text=Gloves-Brown',   100, 1),
    ('glasses_round',   '圆框眼镜',     3, 1, 'https://placehold.co/128x128?text=Glasses-Round',  'https://placehold.co/64x64?text=Glasses-Round',  100, 1),
    ('neck_blue',       '蓝围脖',       4, 1, 'https://placehold.co/128x128?text=Neck-Blue',      'https://placehold.co/64x64?text=Neck-Blue',      100, 1),
    ('back_bag',        '小书包',       5, 1, 'https://placehold.co/128x128?text=Back-Bag',       'https://placehold.co/64x64?text=Back-Bag',       100, 1),
    ('tail_ribbon',     '蝴蝶结尾巾',   7, 1, 'https://placehold.co/128x128?text=Tail-Ribbon',    'https://placehold.co/64x64?text=Tail-Ribbon',    100, 1),
    -- rare（4 件；drop_weight=20）
    ('hat_chef',        '厨师帽',       1, 2, 'https://placehold.co/128x128?text=Hat-Chef',       'https://placehold.co/64x64?text=Hat-Chef',        20, 1),
    ('glasses_star',    '星星眼镜',     3, 2, 'https://placehold.co/128x128?text=Glasses-Star',   'https://placehold.co/64x64?text=Glasses-Star',    20, 1),
    ('neck_scarf_star', '星星围巾',     4, 2, 'https://placehold.co/128x128?text=Neck-Scarf-Star','https://placehold.co/64x64?text=Neck-Scarf-Star', 20, 1),
    ('body_tshirt',     '白T恤',        6, 2, 'https://placehold.co/128x128?text=Body-Tshirt',    'https://placehold.co/64x64?text=Body-Tshirt',     20, 1),
    -- epic（2 件；drop_weight=4）
    ('hat_crown',       '金王冠',       1, 3, 'https://placehold.co/128x128?text=Hat-Crown',      'https://placehold.co/64x64?text=Hat-Crown',        4, 1),
    ('back_wings',      '天使翅膀',     5, 3, 'https://placehold.co/128x128?text=Back-Wings',     'https://placehold.co/64x64?text=Back-Wings',       4, 1),
    -- legendary（1 件；drop_weight=1）
    ('body_armor',      '黄金圣衣',     6, 4, 'https://placehold.co/128x128?text=Body-Armor',     'https://placehold.co/64x64?text=Body-Armor',       1, 1)
ON DUPLICATE KEY UPDATE
    name        = VALUES(name),
    slot        = VALUES(slot),
    rarity      = VALUES(rarity),
    asset_url   = VALUES(asset_url),
    icon_url    = VALUES(icon_url),
    drop_weight = VALUES(drop_weight),
    is_enabled  = VALUES(is_enabled);
