package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultCWDCandidates 是按 CWD 解析的候选配置文件路径，按优先级从高到低。
//   - "server/configs/local.yaml" — 对应"从 repo root 跑 ./build/catserver"的文档化路径
//   - "configs/local.yaml"         — 对应 dev 在 server/ 目录 go run ./cmd/server/
var DefaultCWDCandidates = []string{
	filepath.Join("server", "configs", "local.yaml"),
	filepath.Join("configs", "local.yaml"),
}

// LocateDefault 在一组候选位置里找到第一个存在的配置文件，返回其路径。
// 查找顺序：
//  1. CWD-relative 候选（DefaultCWDCandidates）
//  2. 二进制所在目录 + 常见相对布局（需要 os.Executable 可用）
//
// 设计动机：flag 默认值用 CWD-relative 路径会和 CWD 强耦合，不同启动方式下
// 会踩坑（详见 docs/lessons/2026-04-24-config-path-and-bind-banner.md）。
func LocateDefault() (string, error) {
	return locateIn(DefaultCWDCandidates, executableRelativeCandidates)
}

// locateIn 是 LocateDefault 的可测试核心：参数化候选路径来源以便在 tmp dir 里测试。
func locateIn(cwdCandidates []string, exeCandidatesFn func() []string) (string, error) {
	for _, p := range cwdCandidates {
		if fileExists(p) {
			return p, nil
		}
	}
	if exeCandidatesFn != nil {
		for _, p := range exeCandidatesFn() {
			if fileExists(p) {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("no config file found; tried CWD candidates %v; use -config <path> to set explicitly", cwdCandidates)
}

// executableRelativeCandidates 基于当前二进制所在目录推断候选路径。
// 常见布局：
//   - 二进制 `repo-root/build/catserver.exe` → 配置在 `../server/configs/local.yaml`
//   - 二进制和 configs 同层（装机后 `install/bin` + `install/configs`）→ `./configs/local.yaml`
func executableRelativeCandidates() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	binDir := filepath.Dir(exe)
	return []string{
		filepath.Join(binDir, "..", "server", "configs", "local.yaml"),
		filepath.Join(binDir, "configs", "local.yaml"),
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
