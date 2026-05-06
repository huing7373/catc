-- 对齐 docs/宠物互动App_数据库设计.md §5.14 (行 669-685)
-- room_members 表：房间当前成员表（仅存"当前成员"，不做历史存档）
--
-- **本 migration 由 Story 10.3 review r5 [P1] 引入**（同 0007_init_rooms 背景）：
--   - 把"表存在"从 Epic 11.2 拆出来移到 Story 10.3，让 Story 10.3 self-contained
--     可部署；JOIN / LEAVE / member_count 维护等业务事务仍由 Epic 11.4 / 11.5 落地
--
-- 字段（与 §5.14 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1）
--   - room_id BIGINT UNSIGNED NOT NULL：归属 rooms.id（与 rooms.id 同类型）
--   - user_id BIGINT UNSIGNED NOT NULL：归属 users.id
--   - joined_at / updated_at DATETIME(3)（§3.2）
--
-- 关键约束（§5.14 设计说明 + §7.1 钦定）：
--   - UNIQUE KEY uk_user_id (user_id)：一个用户同一时间**只**在一个房间
--     （§5.14 设计说明明确"一个用户同时只能在一个房间：UNIQUE(user_id)"）
--   - UNIQUE KEY uk_room_user (room_id, user_id)：房间内同一用户只能出现一次
--     （§5.14 设计说明"房间内同一用户只能出现一次：UNIQUE(room_id, user_id)"）
--   - KEY idx_room_id (room_id)：按 room 查全成员（§7.2 高优先级普通索引钦定
--     "room_members(room_id)"）
--
-- **关于 PK / 索引设计差异说明**：
--   - 数据库设计文档 §5.14 钦定 PK 是 AUTO_INCREMENT id（与本表其他业务表一致）
--   - room_member_repo.go 注释里曾写 PK=(room_id, user_id) 复合主键 —— 那是
--     repo 实装注释里早期草稿（Story 10.3 dev 期间），与 §5.14 钦定不一致；
--     本 migration **以数据库设计文档 §5.14 为准**（uk_room_user 是 UNIQUE 约束
--     不是 PK）
--   - 不影响 IsUserInRoom 查询性能：uk_user_id 已覆盖 user_id 单列查询；
--     uk_room_user 已覆盖 (room_id, user_id) 复合查询
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何业务逻辑（JOIN room
-- INSERT / LEAVE room DELETE 事务在 Epic 11.4 / 11.5 落地）。
CREATE TABLE room_members (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    room_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id (user_id),
    UNIQUE KEY uk_room_user (room_id, user_id),
    KEY idx_room_id (room_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
