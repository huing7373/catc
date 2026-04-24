// Package buildinfo 暴露编译期注入的构建元信息（git commit / 构建时间）。
//
// 两个包级变量通过 `go build -ldflags -X` 注入，例如：
//
//	go build \
//	  -ldflags "-X 'github.com/huing/cat/server/internal/buildinfo.Commit=abc1234' \
//	            -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=2026-04-27T10:00:00Z'" \
//	  -o ../build/catserver.exe ./cmd/server/
//
// 未注入时（例如 `go run`、test binary）保持默认 "unknown"，
// 这样 /version 端点返回的 JSON 不会出现空串。
//
// 注意：变量必须是 var 不能是 const —— Go 的 -ldflags -X
// 只能覆盖 var，const 会编译通过但注入静默失效。
package buildinfo

// Commit 是编译期通过 -ldflags -X 注入的 git short hash。
// 未注入时默认 "unknown"。
var Commit = "unknown"

// BuiltAt 是编译期通过 -ldflags -X 注入的 ISO8601 UTC 构建时间戳。
// 未注入时默认 "unknown"。
var BuiltAt = "unknown"
