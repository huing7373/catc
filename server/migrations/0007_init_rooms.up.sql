-- 对齐 docs/宠物互动App_数据库设计.md §5.13 (行 636-651)
-- rooms 表：房间主表
--
-- **本 migration 由 Story 10.3 review r5 [P1] 引入**：
--   - Story 10.3 原计划"不实装 rooms / room_members migration（Epic 11.2 接管）"
--   - r5 review 指出：当前 prod 部署用 0001-0006 起服务时，wsTablesReady() 永远
--     false → /ws/rooms/:roomId 永远不挂 → client 拿到 404（不是 documented WS
--     close codes），Story 10.3 在 prod / smoke test 完全不可用
--   - 修法（review 建议）：把"表存在"与"业务 INSERT/UPDATE 逻辑"分开 ——
--     CREATE TABLE 移到 Story 10.3，让 Story 10.3 self-contained 可部署；JOIN /
--     LEAVE room 等业务逻辑仍由 Epic 11.4 / 11.5 落地
--
-- 字段（与 §5.13 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - creator_user_id BIGINT UNSIGNED NOT NULL：创建者 user.id（与 users.id 同类型）
--   - status TINYINT NOT NULL DEFAULT 1：房间状态（§6.12 钦定 1=active / 2=closed；
--     业务 transition 由 Epic 11.3 / 11.6 实装，本 migration 仅建表 + 默认 active）
--   - max_members TINYINT UNSIGNED NOT NULL DEFAULT 4：最大成员数（MVP 固定 4）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳）
--
-- 索引（§7 钦定）：
--   - KEY idx_creator_user_id (creator_user_id)：未来按创建者查房间列表（Epic 11.x）
--   - KEY idx_status_created_at (status, created_at)：批量查"active 房间按创建时间
--     倒序"或"closed 房间归档清理" 用
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何业务逻辑（JOIN / LEAVE
-- 事务在 Epic 11.4 / 11.5 落地；status transition 在 Epic 11.3 / 11.6 落地）。
CREATE TABLE rooms (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    creator_user_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL DEFAULT 1,
    max_members TINYINT UNSIGNED NOT NULL DEFAULT 4,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    KEY idx_creator_user_id (creator_user_id),
    KEY idx_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
