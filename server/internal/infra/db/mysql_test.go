package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/huing/cat/server/internal/infra/config"
)

// TestOpen_EmptyDSN_ReturnsError 验证 cfg.DSN 为空时 fail-fast：
// 直接返 error 而不调驱动；error 信息含 "dsn" 关键字便于 dev 排错。
//
// 这是节点 2 阶段最常踩的坑：忘记 local.yaml 加 mysql 段 / 环境变量没注入 →
// DSN 为空，必须立即报错而不是用空连接池启动。
func TestOpen_EmptyDSN_ReturnsError(t *testing.T) {
	ctx := context.Background()
	cfg := config.MySQLConfig{
		DSN:                "",
		MaxOpenConns:       50,
		MaxIdleConns:       10,
		ConnMaxLifetimeSec: 1800,
	}

	gormDB, err := Open(ctx, cfg)
	if err == nil {
		t.Fatalf("Open returned nil error for empty DSN, want error")
	}
	if gormDB != nil {
		t.Errorf("Open returned non-nil *gorm.DB on error, want nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "dsn") {
		t.Errorf("error message = %q, want substring %q", err.Error(), "dsn")
	}
}

// TestOpen_InvalidDSN_ReturnsError 验证 cfg.DSN 格式不合法时 fail-fast：
// gorm.Open / driver parse / ping 任一阶段失败都直接返 error。
//
// 用 "notvalid" 作 DSN：go-sql-driver/mysql 在 parse DSN 阶段就会拒绝（缺少
// `@tcp(host:port)/db` 结构）。即使 parse 通过也会在 PingContext 时失败。
func TestOpen_InvalidDSN_ReturnsError(t *testing.T) {
	ctx := context.Background()
	cfg := config.MySQLConfig{
		DSN:                "notvalid",
		MaxOpenConns:       50,
		MaxIdleConns:       10,
		ConnMaxLifetimeSec: 1800,
	}

	gormDB, err := Open(ctx, cfg)
	if err == nil {
		t.Fatalf("Open returned nil error for invalid DSN %q, want error", cfg.DSN)
	}
	if gormDB != nil {
		t.Errorf("Open returned non-nil *gorm.DB on error, want nil")
	}
	// 不强检具体错误消息（gorm / driver 错误措辞会随版本变），只验证 wrap 链路：
	// errors.Unwrap 至少能拿到底层 error（fmt.Errorf("...: %w") 形态）。
	if errors.Unwrap(err) == nil {
		t.Errorf("error %q did not wrap an underlying cause; want fmt.Errorf(\"...: %%w\") form", err.Error())
	}
}

// TestOpen_UnreachableDSN_ReturnsPingError 验证 DSN 格式合法但目标 MySQL 不可达时，
// PingContext 阶段返 error。这是 fail-fast 的核心场景：服务部署时 DB 没准备好，
// server 必须立刻退出而不是假装启动成功。
//
// 用 127.0.0.1:1（特权端口 1，几乎不可能有 MySQL 监听）+ 极短 timeout 让 ping 立刻失败。
// 不依赖 docker / 真 MySQL，纯单元测试。
func TestOpen_UnreachableDSN_ReturnsPingError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout())
	defer cancel()

	cfg := config.MySQLConfig{
		// 端口 1 几乎不会有任何服务监听；timeout 参数让 driver 在 1s 内报错而非默认 90s。
		DSN:                "user:pass@tcp(127.0.0.1:1)/db?timeout=1s&readTimeout=1s&writeTimeout=1s",
		MaxOpenConns:       50,
		MaxIdleConns:       10,
		ConnMaxLifetimeSec: 1800,
	}

	gormDB, err := Open(ctx, cfg)
	if err == nil {
		t.Fatalf("Open returned nil error for unreachable DSN, want ping error")
	}
	if gormDB != nil {
		t.Errorf("Open returned non-nil *gorm.DB on ping failure, want nil")
	}
	// 只断言 error 含 "mysql" 前缀（"mysql open: ..." 或 "mysql ping: ..." 都接受），
	// 不强行断言 "ping" 字面量（GORM 内部可能在 Open 阶段就尝试连接 → 早于 PingContext 失败）。
	if !strings.Contains(strings.ToLower(err.Error()), "mysql") {
		t.Errorf("error message = %q, want substring %q", err.Error(), "mysql")
	}
}
