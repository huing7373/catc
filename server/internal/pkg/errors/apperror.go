// Package apperror 提供全 server 业务错误的统一类型 *AppError 与三层映射工具。
//
// # 包名 vs Import 路径
//
// 本包 import 路径是 `github.com/huing/cat/server/internal/pkg/errors`（对齐
// docs/宠物互动App_Go项目结构与模块职责设计.md §11 + §4 钦定的目录），但
// 包名声明为 `apperror`（**不是** `errors`，避免与 stdlib `errors` 冲突）。
//
// 调用方惯用法：
//
//	import (
//	    stderrors "errors"
//	    apperror "github.com/huing/cat/server/internal/pkg/errors"
//	)
//
// 这样 `apperror.New(...)` / `apperror.Wrap(err, code, msg)` 与 stdlib
// `stderrors.Is` / `stderrors.As` 在同一文件里共存无歧义。
//
// # 三层映射约定（NFR18 / 设计文档 §11）
//
//   - repo 层：返回原生 error（如 GORM `gorm.ErrRecordNotFound`、Redis `redis.Nil`），
//     **不** wrap，保留底层错误链
//   - service 层：catch repo error 后用 `apperror.Wrap(err, code, msg)` 转业务错误，
//     业务码取自本包 codes.go 定义的 26 个常量
//   - handler 层：用 `c.Error(err)` + `c.Abort()` 把错误推给 ErrorMappingMiddleware，
//     由 middleware 翻译成 V1接口设计 §2.4 envelope `{code, message, data, requestId}`
//
// # nil-safety
//
// `Wrap(nil, ...)` 返回真 nil（**不**返回 `*AppError{Cause: nil}`）—— 让 service
// 写 `return apperror.Wrap(s.repo.X(ctx), ...)` 一行就够，不用再加 `if err == nil`
// 短路。`As` / `Code` 接受 nil 输入，分别返回 `(nil, false)` 与 `0`。
//
// # errors.Is / errors.As 穿透
//
// `*AppError.Unwrap()` 返回 `Cause`；调用方可以用 `stderrors.Is(wrappedErr, sql.ErrNoRows)`
// 在 service 层穿透 wrap 链识别底层错误。详见 ADR-0006 §2。
package apperror

import (
	stderrors "errors"
	"fmt"
)

// AppError 是全 server 业务错误的统一类型。
//
// 字段语义：
//   - Code: 业务错误码（V1接口设计 §3，0 = 成功；业务错误用非 0 码）
//   - Message: 给客户端展示的错误文案（中文 hard-coded，不做 i18n）
//   - Cause: 底层原因（repo 返回的 sql.ErrNoRows / GORM 错误等），可 nil
//   - Metadata: 附加诊断字段（如 user_id / request_id），可 nil；
//     写入 logger 时由 middleware 决定是否展开
type AppError struct {
	Code     int
	Message  string
	Cause    error
	Metadata map[string]any
}

// Error 实现 error 接口。格式：
//   - 无 Cause："code=<code> msg=<message>"
//   - 有 Cause："code=<code> msg=<message>: <cause>"
//
// 不做 nil-safe（Go 惯例：nil pointer 调方法 panic）。
// 本包 `New` / `Wrap` 都返回非 nil（`Wrap(nil, ...)` 是特例：返回 nil）。
func (e *AppError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("code=%d msg=%s", e.Code, e.Message)
	}
	return fmt.Sprintf("code=%d msg=%s: %s", e.Code, e.Message, e.Cause.Error())
}

// Unwrap 返回 Cause，让 stderrors.Is / stderrors.As 能穿透 AppError 看到
// 底层错误链（如 service 层 wrap 后 handler 仍能识别 sql.ErrNoRows）。
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithMetadata 在已构造的 AppError 上追加诊断字段（链式调用）。
// 多次调 metadata 累加；同 key 后写覆盖。
//
// 用法：
//
//	return apperror.New(ErrInvalidParam, "userId 必填").
//	    WithMetadata("request_path", c.FullPath())
//
// 返回 receiver 自身（非 deep-copy）—— 链式调用是构造期使用，不期待并发。
func (e *AppError) WithMetadata(key string, value any) *AppError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}

// New 构造一个无 cause 的 AppError。
//
// code 应当是 codes.go 里定义的 26 个常量之一；传入未注册的 code 不会 panic
// （dev 自由），但 review 时会问"为什么不复用 26 码"。
func New(code int, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

// Wrap 在 cause 之上包一层 AppError；保留 cause 用于 stderrors.Is/As 穿透。
//
// **nil-safe**：Wrap(nil, ...) 返回 nil（**不**返回 `*AppError{Cause: nil}`）。
// 这是 epics.md AC 强制的 edge case，让 service 层可以写：
//
//	dto, err := s.repo.FindByID(ctx, id)
//	return dto, apperror.Wrap(err, ErrResourceNotFound, "找不到资源")
//
// err 为 nil 时直接 return nil，不需要在 service 内多一层 if 短路。
//
// 注意：返回类型是 `*AppError`；为了避免 `(*AppError)(nil)` 包成 non-nil
// `error` interface 的经典坑，调用方应当**直接**返回本函数结果给 error 类型
// 的返回值（`return apperror.Wrap(...)`），不要先 assignment 给 *AppError
// 变量再返回。
func Wrap(err error, code int, msg string) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{Code: code, Message: msg, Cause: err}
}

// As 是 stderrors.As 的快捷封装：尝试把 err 链上的某一层断言为 *AppError。
//
// 用于 ErrorMappingMiddleware：c.Errors 里的 error 可能是 *AppError、
// 也可能是被 fmt.Errorf("...: %w", appErr) 包过的多层 wrap —— 用本函数
// 穿透 Unwrap 链找到第一层 *AppError。
//
// 接受 nil 输入：返回 (nil, false)。
func As(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *AppError
	if stderrors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// Code 是从 err 链上提取业务码的快捷方法。
//
// 找不到 *AppError 时返回 0（成功码 = "未识别为业务错误"）。
// 接受 nil 输入：返回 0。
//
// 用于 logger 中间件 / 监控埋点等"想拿 code 就拿 code，没就 0"的场景。
func Code(err error) int {
	if ae, ok := As(err); ok {
		return ae.Code
	}
	return 0
}
