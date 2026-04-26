-- 对齐 docs/宠物互动App_数据库设计.md §5.6 (行 362-395)
-- user_chests 表：当前宝箱表
-- - 一个用户始终只有一个 "当前宝箱"（UNIQUE uk_user_id）
-- - status TINYINT：宝箱状态（§6 钦定 1=锁定/2=可开启 等；本 story 只建表）
-- - unlock_at DATETIME(3)：可开启时间点（倒计时结束后可视为 unlockable）
-- - open_cost_steps INT UNSIGNED DEFAULT 1000：开启所需步数（节点 7 开箱业务消费）
-- - version BIGINT UNSIGNED：乐观锁版本号，防重复开启
-- - idx_status_unlock_at：批量轮询 "已到时间但仍锁定" 的宝箱时使用
CREATE TABLE user_chests (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL,
    unlock_at DATETIME(3) NOT NULL,
    open_cost_steps INT UNSIGNED NOT NULL DEFAULT 1000,
    version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id (user_id),
    KEY idx_status_unlock_at (status, unlock_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
