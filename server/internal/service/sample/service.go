// Package sample 是 service 层的**模板示范**包。
//
// 本包**不承载任何业务语义**，只是为 Epic 4+ 的真实 service（auth / step /
// chest / compose / room 等）提供一个"依赖注入 + mock 单测"的最小骨架。
// 未来新增业务 service 时，**直接复制本包结构**，按 `docs/lessons/` 或 AC 里
// 罗列的 rename 点（包名 / DTO / interface / method / 错误变量）一次改全即可。
//
// 分层约束（与 docs/宠物互动App_Go项目结构与模块职责设计.md §4 对齐）：
//   - repo interface 定义在**本包内**（而不是 `internal/repo/` 的具体实现包）
//     —— service 层拥有自己的 port，repo 层实装方向 port。这样 service 测试
//     只依赖本包的 interface，不需要 import 真 repo 实装。
//   - 返回值用 *DTO（指针）+ error 的二元组，让 `Return(nil, err)` 语义清晰
//     —— 同时也是 testify/mock 约定的标准签名（args.Get(0) 做 comma-ok 断言）。
//   - 所有导出方法第一个参数是 ctx context.Context —— Story 1.9 即将固化为
//     ADR 0007，本包先示范该约定。
package sample

import (
	"context"
	"errors"
)

// ErrSampleNotFound 演示 service 层的业务错误。
//
// 本 story 用 stdlib errors.New 占位；Story 1.8 落地 AppError 框架后，
// 真实 service 的业务错误会替换为 `apperror.New(code, msg)` 形式。
var ErrSampleNotFound = errors.New("sample not found")

// SampleDTO 演示 service 层返回的数据对象。字段命名无业务含义，仅为模板示范。
type SampleDTO struct {
	ID    string
	Value int
}

// SampleRepo 是 SampleService 依赖的 repo 接口。
//
// 本 story 不落地任何真实现；测试时由 testify/mock 手写的 MockSampleRepo
// 实现该接口。Epic 4 Story 4-2 落地 MySQL repo 时，真实 repo 会在
// `internal/repo/mysql/` 下实现对应 domain 的 interface。
type SampleRepo interface {
	FindByID(ctx context.Context, id string) (*SampleDTO, error)
}

// SampleService 演示一个最小 service：依赖 SampleRepo，封装一次查询调用。
type SampleService struct {
	repo SampleRepo
}

// NewSampleService 构造函数。依赖通过参数注入，测试时替换为 mock。
func NewSampleService(repo SampleRepo) *SampleService {
	return &SampleService{repo: repo}
}

// GetValue 演示 service 层的标准调用链：
//   - id 为空串 → 返回 ErrSampleNotFound（预校验，不落 repo）
//   - repo 返回 err → service 原样向上传（Story 1.8 会替换为 apperror.Wrap）
//   - repo 返回 (nil, nil) → 翻译为 ErrSampleNotFound（业务语义"找不到"）
//   - repo 返回 (dto, nil) → 返回 dto.Value
//
// 关于 `(nil, nil)` 的约定（**重要，复制本模板时必须保留**）：
// 常见的 repo 实装会把 `sql.ErrNoRows` / `redis.Nil` / MongoDB "no document"
// 转成 `(nil, nil)` 作为"正常的不存在"语义上传（而不是 err 非 nil）。
// 如果 service 只检查 `err != nil`、不检查 `dto == nil`，下一行
// `dto.Value` 会 panic。本模板**显式**兜底，让 Epic 4+ 复制模板时自然
// 继承该保护；如果真实业务确实需要"找到空对象"语义（少见），重写这段即可。
//
// 本方法刻意保持最小，避免 sample 吸收任何"sample 专属业务"。
// 复制本方法作为模板时，将 id 校验 / repo 调用 / 返回包装三段分别替换为
// 真业务逻辑即可。
func (s *SampleService) GetValue(ctx context.Context, id string) (int, error) {
	if id == "" {
		return 0, ErrSampleNotFound
	}
	dto, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return 0, err
	}
	if dto == nil {
		return 0, ErrSampleNotFound
	}
	return dto.Value, nil
}
