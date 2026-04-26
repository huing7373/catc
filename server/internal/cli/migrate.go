// Package cli 实装 catserver 的子命令分发逻辑。当前仅 migrate；未来可扩展（dev /
// dump-schema / etc）。从 main.go 抽出来是为了让单测可对子命令分发逻辑做表驱动覆盖
// （main.go 是 package main 难以单测）。
//
// # 设计
//
//   - cli.Migrator interface 与 internal/infra/migrate.Migrator 同签名 ——
//     RunMigrate 内部 New 真实 Migrator 然后传给 runMigrateAction（接口注入）
//   - runMigrateAction 只接 Migrator interface，单测构造 fake Migrator 即可全覆盖
//     action 分发 / exit code 决策逻辑
//   - dirty 状态视为 unhealthy：Status 返 dirty=true 时 RunMigrate 返 error
//     让 main.go os.Exit(1)，方便 CI 感知
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/migrate"
)

// Migrator 是 cli 包内部的 migration 抽象，签名与 internal/infra/migrate.Migrator 完全一致。
//
// 故意复制一份 interface 定义（而非直接 import migrate.Migrator）：
//   - 让 runMigrateAction 单测可注入 fake Migrator 不需要 import 整个 migrate 包
//   - 主流程在 RunMigrate 里 type assert（实装接口的 *migrate.migrator 自动满足）
type Migrator interface {
	Up(ctx context.Context) error
	Down(ctx context.Context) error
	Status(ctx context.Context) (uint, bool, error)
	Close() error
}

// ParseMigrateArgs 解析 `migrate` 子命令之后的参数（不含 "migrate" 本身），
// 返回 (action, configPathOverride, error)。
//
// 支持三种形态（review fix Story 4.3）：
//
//   - `migrate up`                              → action="up", override=""
//   - `migrate -config configs/dev.yaml up`     → action="up", override="configs/dev.yaml"
//   - `migrate up -config configs/dev.yaml`     → action="up", override="configs/dev.yaml"
//
// Go 的 flag 包在第一个非 flag 参数处停止解析（不支持 flag 出现在 positional 之后），
// 所以本函数先把 args 拆成 flagsAndAction —— 把第一个 positional（action）拎出来后，
// 剩余参数交给 FlagSet.Parse。这样无论 -config 在前还是在后都能正确解析。
//
// errOutput 是 FlagSet 的 Output（解析错误日志）—— 测试可注入 buffer，生产传 os.Stderr 或 io.Discard。
func ParseMigrateArgs(args []string, errOutput io.Writer) (action, configPathOverride string, err error) {
	if len(args) == 0 {
		return "", "", errors.New("migrate requires action: up / down / status")
	}

	// 把 args 拆成 (flagPart, action)：扫描 args 找到第一个不以 "-" 开头的 token 作为 action。
	// 注意要识别 "-config foo" 这种 flag-with-value（value 不能误判为 action）。
	//
	// 已知 flag：-config（带 value）。其他 flag 一概当作不带 value（FlagSet 会兜底报错）。
	// 简单可控的策略：手工识别 -config，其他 token 全交给 FlagSet 处理。
	knownFlagsWithValue := map[string]bool{"-config": true, "--config": true}

	var flagPart []string
	actionFound := ""
	i := 0
	for i < len(args) {
		tok := args[i]
		if knownFlagsWithValue[tok] {
			// 带 value 的 flag：把当前 token + 下一个 token 一起放进 flagPart
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("migrate: flag %s requires a value", tok)
			}
			flagPart = append(flagPart, tok, args[i+1])
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "-") {
			// 其他 flag（含 -config=foo 形态）：原样塞进 flagPart 让 FlagSet 解析
			flagPart = append(flagPart, tok)
			i++
			continue
		}
		// 非 flag：第一次出现 = action；再次出现 = 多余 positional → error
		if actionFound != "" {
			return "", "", fmt.Errorf("migrate accepts a single action, got extra positional %q after %q", tok, actionFound)
		}
		actionFound = tok
		i++
	}

	if actionFound == "" {
		return "", "", errors.New("migrate requires action: up / down / status")
	}

	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	if errOutput != nil {
		fs.SetOutput(errOutput)
	}
	fs.StringVar(&configPathOverride, "config", "", "path to config YAML override for this migrate run")
	if perr := fs.Parse(flagPart); perr != nil {
		return "", "", fmt.Errorf("migrate flag parse: %w", perr)
	}
	if rest := fs.Args(); len(rest) > 0 {
		// 不应到这里 —— 我们已把 positional 全拎出来了
		return "", "", fmt.Errorf("migrate: unexpected residual positional args %v", rest)
	}
	return actionFound, configPathOverride, nil
}

// RunMigrate 是 catserver migrate {up|down|status} 子命令的入口。
//
//   - args = "migrate" 之后的全部参数（含可能的 -config flag）
//   - 内部用 ParseMigrateArgs 拆 action + -config override
//   - 若 args 含 -config 且与 cfg 加载路径不同 → 重新 Load 该 config 并覆盖（review fix）
//   - migrationsPath 默认 "migrations"，可被 env CAT_MIGRATIONS_PATH 覆盖（局部开关，
//     不入 cfg.Config —— Story 4.3 决策）
//
// 错误返回时调用方（main.go）打 slog.Error + os.Exit(1)。
// status 返回 dirty=true 时也返 error（让 CI 能感知 schema 处于 dirty 状态）。
func RunMigrate(ctx context.Context, cfg *config.Config, args []string) error {
	if cfg == nil {
		return errors.New("RunMigrate: cfg is nil")
	}

	action, configOverride, err := ParseMigrateArgs(args, os.Stderr)
	if err != nil {
		return err
	}

	// 子命令位置的 -config 优先（更靠近 action 的语义胜出）。重新 Load + 覆盖 cfg。
	// 这条路径覆盖 `catserver migrate up -config configs/dev.yaml` 的文档化形态。
	if configOverride != "" {
		newCfg, lerr := config.Load(configOverride)
		if lerr != nil {
			return fmt.Errorf("migrate: load -config %q: %w", configOverride, lerr)
		}
		cfg = newCfg
		slog.Info("migrate config override", slog.String("path", configOverride))
	}

	migrationsPath := os.Getenv("CAT_MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "migrations"
	}

	mig, err := migrate.New(cfg.MySQL.DSN, migrationsPath)
	if err != nil {
		// migrate.New 内部已 wrap 一层 "migrate.New: ..."，这里再包会出现重复前缀。
		// 直接透传内部 error，错误源信息已足够。
		return err
	}
	defer func() {
		if cerr := mig.Close(); cerr != nil {
			slog.Warn("migrate close failed", slog.Any("error", cerr))
		}
	}()

	slog.Info("migrate started", slog.String("action", action), slog.String("path", migrationsPath))
	if err := runMigrateAction(ctx, mig, action); err != nil {
		return err
	}
	slog.Info("migrate finished", slog.String("action", action))
	return nil
}

// runMigrateAction 是可被 mock 注入测试的核心分发函数。接 Migrator 接口而非
// 具体类型，让单测注入 fake。
//
//   - up / down：直接调对应方法，error 透传
//   - status：调 Status → 打日志 → dirty=true 时返 error（CI 视为 unhealthy）
//   - 其他：返 error("unknown migrate action: %s")
func runMigrateAction(ctx context.Context, mig Migrator, action string) error {
	switch action {
	case "up":
		return mig.Up(ctx)
	case "down":
		return mig.Down(ctx)
	case "status":
		version, dirty, err := mig.Status(ctx)
		if err != nil {
			return err
		}
		// stdout 单行明文（运维 grep 友好）+ 结构化 slog.Info（JSON ops log）双重输出
		fmt.Fprintf(os.Stdout, "migrate: version=%d dirty=%t\n", version, dirty)
		slog.Info("migrate status", slog.Uint64("version", uint64(version)), slog.Bool("dirty", dirty))
		if dirty {
			return fmt.Errorf("schema is dirty at version %d (manual fix required)", version)
		}
		return nil
	default:
		return fmt.Errorf("unknown migrate action: %s (expected up / down / status)", action)
	}
}
