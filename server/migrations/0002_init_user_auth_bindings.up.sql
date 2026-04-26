-- 对齐 docs/宠物互动App_数据库设计.md §5.2 (行 210-243)
-- user_auth_bindings 表：用户登录绑定关系（游客 / 微信 / 后续登录方式）
-- - 游客场景：auth_type=1（guest）+ auth_identifier=guest_uid
-- - 关键约束：UNIQUE (auth_type, auth_identifier) 保证同一登录身份只绑一个账号
-- - auth_extra JSON NULL：扩展登录信息（如微信附加资料）
CREATE TABLE user_auth_bindings (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    auth_type TINYINT NOT NULL,
    auth_identifier VARCHAR(128) NOT NULL,
    auth_extra JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_auth_type_identifier (auth_type, auth_identifier),
    KEY idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
