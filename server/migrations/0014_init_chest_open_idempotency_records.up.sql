-- 对齐 docs/宠物互动App_数据库设计.md §5.16
-- chest_open_idempotency_records 表：开箱接口幂等记录（20.1 r5/r6/r7/r11 锁定 DB 持久化方案，
--   预声明 + 业务写入 + 最终化全部同事务原子提交）
-- 详见 V1接口设计 §7.2 服务端逻辑步骤 3 / 5a / 5b / 5k / 7 + 关键约束「事务边界」+
--   「r7 移除 best-effort failed upsert 决策」+「MVCC 下 pending 不可见 r11 决策」+
--   「1008 错误码 r11 退役决策」段。
--
-- **本 migration 由 Story 20.6 落地**（20.1 r5 follow-up 钦定的 owner 决策）：
--   - 选项 A 采纳：与 service / handler 实装在同 story 一并落地，紧耦合 = AC 边界天然合体
--   - 集成测试不可分：本 story dockertest case 必须有此表才能跑（事务首条语句 INSERT）
--
-- 字段（与 §5.16 钦定 1:1 对齐）：
--   - id: BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - user_id: BIGINT UNSIGNED NOT NULL（归属用户 id，语义上 ref users.id，**不**建 FK）
--   - idempotency_key: VARCHAR(128) NOT NULL（client 传入幂等键；V1 §7.2 钦定 [A-Za-z0-9_:-] + length 1-128）
--   - status: ENUM('pending', 'success') NOT NULL DEFAULT 'pending'
--       **二态机**（r7 锁定，从 r6 三态机简化）；**无** 'failed' 状态
--       pending: 业务事务持锁执行中（InnoDB MVCC 决定对其他事务的 autocommit SELECT 不可见）
--       success: 业务事务已 commit，response_json 已落盘
--   - response_json: JSON NULL
--       status='success' 时存 V1 §7.2 钦定缓存内容（{code, message, data: {reward, stepAccount, nextChest.{id, unlockAt, openCostSteps}}}）
--       **不**含 nextChest.status / nextChest.remainingSeconds（time-derived 字段；cached replay 时同源同时刻重算）
--       **不**含顶层 requestId（每次请求独立 trace ID；重试请求填本次）
--       status='pending' 时为 NULL
--   - created_at / updated_at: 标准毫秒时间戳；updated_at 在 status 推进时自动更新
--
-- 索引（§5.16 钦定）：
--   - UNIQUE KEY uk_user_id_key (user_id, idempotency_key): 兼任原子声明依据 + 并发阻塞排队依据
--       V1 §7.2.5a 用 INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id) 借此做 single-statement 原子 claim
--       InnoDB unique-key X-lock 让同 key 并发请求 serialize 排队（首事务结束前其他事务 INSERT 阻塞）
--   - KEY idx_status_created_at (status, created_at): 辅助索引，运维清理任务按 status + created_at 范围扫描
--       （MVP 阶段无需主动清理；future 容量增长时按此索引清理 N 天前的 success 记录）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据（运行时表，无 seed 阶段）；
--   不含其他表改动；不含 FK 约束（与本设计其他表一致）。
CREATE TABLE chest_open_idempotency_records (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    status ENUM('pending', 'success') NOT NULL DEFAULT 'pending',
    response_json JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id_key (user_id, idempotency_key),
    KEY idx_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
