-- 对齐 docs/宠物互动App_数据库设计.md §5.5 (行 317-359)
-- user_step_sync_logs 表：步数同步日志（节点 3 步数业务核心审计 / 增量计算依据）
-- - id BIGINT UNSIGNED AUTO_INCREMENT 主键（§3.1）
-- - user_id BIGINT UNSIGNED NOT NULL：归属用户
-- - sync_date DATE NOT NULL：客户端按本机时区算出的"今天"，server 直接采用不二次转换
--   （V1 §6.1.2 syncDate 字段说明 + GAP E 时区契约）
-- - client_total_steps BIGINT UNSIGNED NOT NULL：客户端读取到的"当天系统累计步数"
--   （非增量；增量由 server 按上次同步差值计算 —— V1 §6.1.4 服务端逻辑 + §8.2 步数同步事务）
-- - accepted_delta_steps INT NOT NULL DEFAULT 0：服务端实际确认入账的增量
--   （可能因截断 5000 / 当日封顶 50000 / 倒退 < clientTotalSteps 而 ≠ "client - last"，
--    防作弊语义对齐 V1 §6.1.4 + epics.md Story 7.3 GAP K）
-- - motion_state TINYINT NOT NULL DEFAULT 1：同步时客户端活动状态
--   （§6.5 钦定 1=stationary_or_unknown / 2=walking / 3=running）
-- - source TINYINT NOT NULL DEFAULT 1：步数来源
--   （§6.6 钦定 1=healthkit（客户端正常上报） / 2=admin_grant（Story 7.5 dev/运营手动发放））
-- - client_ts BIGINT UNSIGNED NOT NULL DEFAULT 0：客户端调用接口时的本机毫秒时间戳
--   （仅写日志审计用，不参与差值计算 —— V1 §6.1.2 clientTimestamp 字段说明）
-- - created_at DATETIME(3)：服务端写入时间（毫秒精度，§3.2）
-- 索引（§7.2 钦定）：
-- - idx_user_date (user_id, sync_date)：服务端差值计算查"最近一条"用（V1 §6.1.4 / §8.2）
-- - idx_user_created_at (user_id, created_at)：审计 / 时间序追溯（按用户按时间倒序查）
CREATE TABLE user_step_sync_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    sync_date DATE NOT NULL,
    client_total_steps BIGINT UNSIGNED NOT NULL,
    accepted_delta_steps INT NOT NULL DEFAULT 0,
    motion_state TINYINT NOT NULL DEFAULT 1,
    source TINYINT NOT NULL DEFAULT 1,
    client_ts BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_date (user_id, sync_date),
    KEY idx_user_created_at (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
