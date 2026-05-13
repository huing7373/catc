-- 对齐 docs/宠物互动App_数据库设计.md §5.15 (行 700-718)
-- emoji_configs 表：系统表情配置表
--
-- **本 migration 由 Story 17.2 首次落地（Epic 17 节点 6 表情广播链路 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 / 字段类型 / uk_code + idx_enabled_sort 索引符合 §5.15）
-- + dockertest 集成测试覆盖 UNIQUE KEY uk_code (code) 运行时 INSERT 拒绝行为
-- （epics.md §Story 17.2 钦定的"集成测试覆盖：尝试 INSERT 重复 code → 数据库拒绝"路径）。
--
-- 字段（与 §5.15 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - code VARCHAR(64) NOT NULL：表情业务标识符（如 wave / love / laugh / cry）；
--     与 V1 接口设计 §11.1 / §12.2 / §12.3 锚定的 emojiCode 字段同语义；
--     UNIQUE KEY uk_code 保证全局唯一（Story 17.3 seed 用 INSERT IGNORE
--     兜底重复执行）
--   - name VARCHAR(64) NOT NULL：表情中文名（如 挥手 / 爱心 / 大笑 / 哭泣），
--     client 用作 UI 展示文字
--   - asset_url VARCHAR(255) NOT NULL DEFAULT ''：表情资源 URL；
--     **注**：DDL DEFAULT '' 是兜底语义（避免 admin / dev 临时写入时漏字段），
--     enabled 表情（is_enabled=1）必须有非空 asset_url —— Story 17.3 seed 钦定
--     每个表情非空，server seed 层 / admin 写入层应校验
--   - sort_order INT NOT NULL DEFAULT 0：表情显示顺序（升序）；
--     与 V1 §11.1 data.items[].sortOrder 同语义；
--     INT 是带符号 32 位整数（范围 -2^31 ~ 2^31-1），与 §11.1 字段范围
--     0 ≤ value ≤ 2^31 - 1 兼容（业务不用负值）
--   - is_enabled TINYINT NOT NULL DEFAULT 1：是否启用（§6 枚举：0=disabled / 1=enabled）；
--     §11.1 GET /emojis 仅返回 is_enabled=1 的表情；§12.2 emoji.send 仅校验
--     is_enabled=1 的表情，disabled 表情触发 error 7001（与 emojiCode 不存在
--     合并到同一错误码）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳）
--
-- 索引（§5.15 + §7 钦定）：
--   - UNIQUE KEY uk_code (code)：保证 code 全局唯一（§7.1 高优先级 UNIQUE 约束）；
--     Story 17.3 seed 用 INSERT IGNORE 兜底重复执行；Story 17.5 校验 emojiCode
--     合法性时单列查询命中
--   - KEY idx_enabled_sort (is_enabled, sort_order)：覆盖
--     "SELECT ... WHERE is_enabled=1 ORDER BY sort_order ASC" 路径
--     （§11.1 GET /emojis 服务端逻辑步骤 2 SQL；多列复合索引覆盖筛选 + 排序）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （seed 由 Story 17.3 `0010_seed_emoji_configs.up.sql` 落地，**不**塞进本文件）；
-- 不含任何业务 service / handler / repo write 方法（17.3 ~ 17.5 落地）。
CREATE TABLE emoji_configs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    asset_url VARCHAR(255) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    is_enabled TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_code (code),
    KEY idx_enabled_sort (is_enabled, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
