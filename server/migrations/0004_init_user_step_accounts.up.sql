-- 对齐 docs/宠物互动App_数据库设计.md §5.4 (行 280-313)
-- user_step_accounts 表：步数账户总表
-- - **PK = user_id**（不是自增 id）：1:1 关联 users，账户模型
-- - total_steps / available_steps / consumed_steps 三柱式记账
-- - version BIGINT UNSIGNED：乐观锁版本号，扣减并发保护
--   （Story 4.6 游客登录初始化时 INSERT 默认 version=0；后续扣减事务用乐观锁 +1）
CREATE TABLE user_step_accounts (
    user_id BIGINT UNSIGNED PRIMARY KEY,
    total_steps BIGINT UNSIGNED NOT NULL DEFAULT 0,
    available_steps BIGINT UNSIGNED NOT NULL DEFAULT 0,
    consumed_steps BIGINT UNSIGNED NOT NULL DEFAULT 0,
    version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
