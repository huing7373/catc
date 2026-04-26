//go:build tools
// +build tools

// Package tools 通过 blank import 把"工具类"依赖 pin 到 go.mod，避免 `go mod tidy`
// 把未在生产代码 import 的库剔除（标准 Go 项目模式，参考 kubernetes / cobra）。
//
// 当前 pin 的工具：
//   - github.com/golang-migrate/migrate/v4：Story 4.2 ADR-0003 选定的 migration 工具，
//     Story 4.3 使用 CLI 落 5 张表 SQL 文件 + 跑 up/down。本文件保证版本被 go.mod 锁住，
//     即便 Story 4.3 落地前没有任何生产代码 import。
//
// build tag `tools` 隔离 → 永远不会被生产构建 / 单元测试 / 集成测试 / vet 编译，
// 只是给 `go mod` 看的"我需要这个依赖"声明。
package tools

import (
	_ "github.com/golang-migrate/migrate/v4"
)
