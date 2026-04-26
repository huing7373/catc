-- 对齐 docs/宠物互动App_数据库设计.md §5.3 (行 246-277)
-- pets 表：宠物表（节点 2 阶段每个用户首次登录创建一只默认猫）
-- - pet_type TINYINT DEFAULT 1：当前固定猫
-- - current_state TINYINT DEFAULT 1：最近一次同步的宠物状态（§6 状态枚举）
-- - is_default TINYINT DEFAULT 1：是否默认宠物
-- - 关键约束：UNIQUE (user_id, is_default) 保证同一用户的默认宠物唯一
--   （注意：MVP 阶段 is_default 取值仅 0/1；如要支持多默认场景需重设计该约束）
CREATE TABLE pets (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    pet_type TINYINT NOT NULL DEFAULT 1,
    name VARCHAR(64) NOT NULL DEFAULT '',
    current_state TINYINT NOT NULL DEFAULT 1,
    is_default TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_default_pet (user_id, is_default),
    KEY idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
