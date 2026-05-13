package service

import (
	"context"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// EmojiService 是 emojis handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 home_service / room_service 同模式。
type EmojiService interface {
	// ListAvailable 返回所有 is_enabled=1 的 emoji（V1 §11.1 服务端逻辑钦定）。
	//
	// 流程：
	//  1. emojiRepo.List(ctx) → []mysql.EmojiConfig（仅 is_enabled=1 已被 repo 层 SQL 过滤）
	//  2. DTO 转换：mysql.EmojiConfig → EmojiBrief（裁掉 id / is_enabled / created_at /
	//     updated_at；client 不需要这些字段）
	//  3. 0 行 → []EmojiBrief{}（**永远**非 nil；让 handler / wire 层下发 `items: []`
	//     而非 `null`，与 V1 §11.1 钦定一致）
	//
	// 错误约定：
	//   - emojiRepo.List 失败（含 DB 异常 / 连接断 / 慢查询超时等）→ apperror.Wrap 包成
	//     1009 ErrServiceBusy（与 home_service 4 repo 失败兜底同模式 +
	//     lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）
	//
	// **不**做空字符串过滤：V1 §11.1 钦定 assetUrl 必非空字符串（17.3 seed 已保证 +
	// 17-1 r2 Lesson 3 钦定 server 端 seed 层 / admin 写入层负责校验）；本方法
	// **不**做 `if AssetURL == "" 跳过` 分支 —— 让意外有空 assetUrl 的 enabled 行
	// 透传到 client 触发渲染失败而不是被 server 静默过滤（与"server 是表情数据 single
	// source of truth"语义一致）
	ListAvailable(ctx context.Context) ([]EmojiBrief, error)
}

// EmojiBrief 是 V1 §11.1 data.items[] 的 service 层映射（**不是** wire DTO，
// handler 转换为 §11.1 钦定 wire 格式：snake_case 字段名 → camelCase）。
//
// 字段（与 §11.1 data.items[] 钦定字段集 1:1 对齐）：
//   - Code:      string（V1 §11.1 `data.items[].code`；client 业务标识符）
//   - Name:      string（V1 §11.1 `data.items[].name`；UI 展示文字）
//   - AssetURL:  string（V1 §11.1 `data.items[].assetUrl`；资源 URL 非空字符串）
//   - SortOrder: int32（V1 §11.1 `data.items[].sortOrder`；显示顺序升序）
//
// **不**含 ID / IsEnabled / CreatedAt / UpdatedAt：V1 §11.1 钦定 client 不需要这些字段。
type EmojiBrief struct {
	Code      string
	Name      string
	AssetURL  string
	SortOrder int32
}

// emojiServiceImpl 是 EmojiService 的默认实装。
type emojiServiceImpl struct {
	emojiRepo mysql.EmojiRepo
}

// NewEmojiService 构造 EmojiService。
func NewEmojiService(emojiRepo mysql.EmojiRepo) EmojiService {
	return &emojiServiceImpl{emojiRepo: emojiRepo}
}

// ListAvailable 实装：单 repo query + DTO 转换 + nil slice 兜底。
func (s *emojiServiceImpl) ListAvailable(ctx context.Context) ([]EmojiBrief, error) {
	rows, err := s.emojiRepo.List(ctx)
	if err != nil {
		// V1 §11.1 错误码表：DB 异常 → 1009 ErrServiceBusy
		// （lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 永远返非 nil slice（即便 rows 是空）—— 让 handler / wire 层下发 `items: []`
	// 而非 `null`（V1 §11.1 钦定 + 行 1833 "items: [] 与 items: null 语义不同"）。
	briefs := make([]EmojiBrief, 0, len(rows))
	for _, r := range rows {
		briefs = append(briefs, EmojiBrief{
			Code:      r.Code,
			Name:      r.Name,
			AssetURL:  r.AssetURL,
			SortOrder: r.SortOrder,
		})
	}
	return briefs, nil
}
