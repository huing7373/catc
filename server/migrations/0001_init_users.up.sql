-- 对齐 docs/宠物互动App_数据库设计.md §5.1 (行 173-207)
-- users 表：用户主表（节点 2 游客身份起点）
-- - id BIGINT UNSIGNED AUTO_INCREMENT 主键（§3.1）
-- - guest_uid VARCHAR(128) UNIQUE：客户端在 Keychain 持久化的游客身份标识
-- - status TINYINT：账号状态枚举（§6 钦定 1=active；本 story 不展开枚举）
-- - current_room_id BIGINT UNSIGNED NULL：当前房间快照字段（首页查询性能）
-- - created_at / updated_at DATETIME(3) 毫秒精度（§3.2）
CREATE TABLE users (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    guest_uid VARCHAR(128) NOT NULL,
    nickname VARCHAR(64) NOT NULL DEFAULT '',
    avatar_url VARCHAR(255) NOT NULL DEFAULT '',
    status TINYINT NOT NULL DEFAULT 1,
    current_room_id BIGINT UNSIGNED NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_guest_uid (guest_uid),
    KEY idx_current_room_id (current_room_id),
    KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
