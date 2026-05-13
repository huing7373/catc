package service

import (
	"context"
	"regexp"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// emojiCodePattern 是 V1 §12.2 字段表 emojiCode 字符集 + 长度约束（17.1 r2 锁定）。
//
// 规则：1 ≤ length ≤ 64 字符；只允许 [a-z0-9_-]；与 §11.1 emoji_configs.code
// VARCHAR(64) + UNIQUE KEY uk_code 一致（大小写敏感）。
//
// **包级 var 而非函数内构造**：regexp.Compile 不便宜，每次 ValidateCode 调用都
// recompile 是浪费；包级 var 编译一次复用，与既有 auth_service 同模式。
var emojiCodePattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

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

	// ValidateCode 校验 emojiCode 合法性（Story 17.5 引入；V1 §12.2 服务端逻辑步骤
	// 2 + 4 合并钦定）。
	//
	// 校验链（按顺序，任一失败立即返）：
	//   1. **字符集 / 长度校验**（V1 §12.2 字段表 emojiCode：`1 ≤ length ≤ 64` +
	//      `[a-z0-9_-]`）：不通过 → apperror.New(ErrInvalidParam /* 1002 */, ...)
	//   2. **DB 存在性校验**（调 emojiRepo.Exists(ctx, code)）：
	//      - err != nil → apperror.Wrap(err, ErrServiceBusy /* 1009 */, ...)
	//      - false（不存在 / disabled）→ apperror.New(ErrEmojiNotFound /* 7001 */, ...)
	//      - true → 返 nil（校验通过）
	//
	// **返 error 而非 bool**：让 handler 走单一 `errors.As` 分支取业务码，避免
	// "service 层返 (bool, code) + handler 走 switch code" 那种 fragile 路径。
	//
	// **不**做 trim / lowercase 等归一化：emojiCode 严格按 client 发送的原始字符串
	// 校验；client 传 "Wave"（大写 W）→ 1002（字符集不允许大写）；这是契约层钦定
	// （§11.1 行 1771 emoji_configs.code 严格 [a-z0-9_-] + UNIQUE KEY 大小写敏感）。
	//
	// **不**做 nil-context / nil-string 防御性 check：调用方（emoji_handler.HandleEmojiSend）
	// 已保证 ctx 来自 session.ctx + code 来自 envelope.payload 解析（解析失败前置
	// 拦截）；这里冗余 nil-check 让 happy path 多两次比较开销，与 ADR-0006 / ADR-0007
	// 同模式（service 层信任 handler 层入参）。
	ValidateCode(ctx context.Context, code string) error
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

// ValidateCode 实装严格按 EmojiService.ValidateCode 注释流程。
func (s *emojiServiceImpl) ValidateCode(ctx context.Context, code string) error {
	// (1) 字符集 / 长度校验（V1 §12.2 字段表）
	if !emojiCodePattern.MatchString(code) {
		return apperror.New(apperror.ErrInvalidParam,
			apperror.DefaultMessages[apperror.ErrInvalidParam])
	}

	// (2) DB 存在性校验（lesson 2026-05-13 Lesson 2 钦定 DB error 必走 1009 路径）
	exists, err := s.emojiRepo.Exists(ctx, code)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrServiceBusy,
			apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if !exists {
		return apperror.New(apperror.ErrEmojiNotFound,
			apperror.DefaultMessages[apperror.ErrEmojiNotFound])
	}

	return nil
}
