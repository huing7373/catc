package service

import (
	"context"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// CosmeticService 是 cosmetics handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 emoji_service /
// home_service / room_service 同模式。
//
// Story 23.3 阶段唯一方法 ListCatalog（GET /api/v1/cosmetics/catalog）；
// future Story 23.4 落地 GET /cosmetics/inventory 时再补 inventory 聚合方法
// （YAGNI；本 story **不**预实装 inventory —— 那是 23.4 钦定范围）。
type CosmeticService interface {
	// ListCatalog 返回所有 is_enabled=1 的 cosmetic_items 配置（V1 §8.1 服务端
	// 逻辑钦定）。
	//
	// 流程（与 emoji_service.ListAvailable 1:1 同模式）：
	//  1. cosmeticItemRepo.ListEnabledForCatalog(ctx) → []mysql.CosmeticItem
	//     （仅 is_enabled=1 已被 repo 层 SQL 过滤 + 按 rarity ASC, slot ASC,
	//     id ASC 三级全序排序）
	//  2. DTO 转换：mysql.CosmeticItem → CosmeticBrief（裁掉 drop_weight /
	//     is_enabled / created_at / updated_at；§8.1 钦定 client 不需要这些字段）
	//  3. 0 行 → []CosmeticBrief{}（**永远**非 nil；让 handler / wire 层下发
	//     `items: []` 而非 `null`，与 §8.1 行 1301 "catalog 为空返 {items:[]}
	//     code=0 不报错" 一致）
	//
	// 错误约定：
	//   - cosmeticItemRepo.ListEnabledForCatalog 失败（含 DB 异常 / 连接断 /
	//     慢查询超时等）→ apperror.Wrap 包成 1009 ErrServiceBusy（与
	//     emoji_service ListAvailable 同模式 + lesson 2026-05-13 Lesson 2 钦定
	//     DB error 必须有 1009 路径）。
	//   - **不**触发 1002（§8.1 行 1301：GET 无 body / 无 query 参数，无可校验
	//     输入）；auth(1001) / rate_limit(1005) 由 router authedGroup 中间件兜底，
	//     service 层不感知。
	//
	// **不**做空字符串过滤：§8.1 行 1267-1268 钦定 enabled cosmetic 的 iconUrl /
	// assetUrl 必非空字符串（Story 20.3 0012 seed 已保证 + admin 写入层负责
	// 校验）；本方法**不**做 `if IconURL == "" 跳过` 分支 —— 让意外有空 URL 的
	// enabled 行透传到 client 触发渲染失败而不是被 server 静默过滤（与"server 是
	// cosmetic 数据 single source of truth"语义一致，与 emoji_service.go 行
	// 38-42 钦定同源）。
	//
	// **范围红线**：本 story（23.3）仅查 cosmetic_items 配置表（§5.8），**不**读
	// userID / **不**查 23.2 落地的 user_cosmetic_items 实例表（§5.9）/ **不**做
	// 任何 user 维度聚合（catalog = 全局静态目录，与 user 无关；inventory 是
	// Story 23.4 钦定范围）。
	ListCatalog(ctx context.Context) ([]CosmeticBrief, error)
}

// CosmeticBrief 是 V1 §8.1 data.items[] 的 service 层映射（**不是** wire DTO，
// handler 转换为 §8.1 钦定 wire 格式：cosmeticItemId 字符串化 + 字段名 camelCase）。
//
// 字段（与 §8.1 data.items[] 钦定 7 字段集 1:1 对齐，行 1260-1268）：
//   - CosmeticItemID: uint64（§8.1 `cosmeticItemId`；handler 层 strconv.FormatUint
//     字符串化 → 与 §2.5 BIGINT 字符串化全局约定 + cosmetic_items.id BIGINT
//     UNSIGNED 一致）
//   - Code:           string（§8.1 `code`；全局唯一业务编码）
//   - Name:           string（§8.1 `name`；装扮中文名 UI 展示文字）
//   - Slot:           int8（§8.1 `slot`，§6.8 枚举 {1,2,3,4,5,6,7,99}；handler
//     层 int 直接下发，**不**字符串化）
//   - Rarity:         int8（§8.1 `rarity`，§6.9 枚举 {1,2,3,4}；handler 层 int
//     直接下发，**不**字符串化）
//   - IconURL:        string（§8.1 `iconUrl`；小尺寸预览图 URL 非空字符串）
//   - AssetURL:       string（§8.1 `assetUrl`；装扮资源 URL 非空字符串）
//
// **不**含 DropWeight / IsEnabled / CreatedAt / UpdatedAt：§8.1 钦定 client 不
// 需要这些字段（与 emoji_service EmojiBrief 裁字段同模式）。
type CosmeticBrief struct {
	CosmeticItemID uint64
	Code           string
	Name           string
	Slot           int8
	Rarity         int8
	IconURL        string
	AssetURL       string
}

// cosmeticServiceImpl 是 CosmeticService 的默认实装。
type cosmeticServiceImpl struct {
	cosmeticItemRepo mysql.CosmeticItemRepo
}

// NewCosmeticService 构造 CosmeticService。Story 23.3 引入。
//
// 注入 CosmeticItemRepo（Story 20.6 既有 interface，23.3 扩了
// ListEnabledForCatalog）—— router wire 复用 line 486 既有 cosmeticItemRepo
// 实例（**不**新建第二个实例，与 chestSvc 复用同实例同模式）。
func NewCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo) CosmeticService {
	return &cosmeticServiceImpl{cosmeticItemRepo: cosmeticItemRepo}
}

// ListCatalog 实装：单 repo query + DTO 转换 + nil slice 兜底。
// 详见 CosmeticService.ListCatalog 接口注释（§8.1 服务端逻辑钦定）。
func (s *cosmeticServiceImpl) ListCatalog(ctx context.Context) ([]CosmeticBrief, error) {
	rows, err := s.cosmeticItemRepo.ListEnabledForCatalog(ctx)
	if err != nil {
		// V1 §8.1 错误码表行 1299：DB 异常 → 1009 ErrServiceBusy
		// （lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 永远返非 nil slice（即便 rows 是空）—— 让 handler / wire 层下发 `items: []`
	// 而非 `null`（V1 §8.1 行 1301：catalog 为空返 {items:[]} code=0 不报错）。
	// **不**做空 URL 过滤（§8.1 行 1267-1268 钦定 enabled 必非空，0012 seed 已
	// 保证；server 透传真实 row，与 emoji_service 行 38-42 同源钦定）。
	briefs := make([]CosmeticBrief, 0, len(rows))
	for _, r := range rows {
		briefs = append(briefs, CosmeticBrief{
			CosmeticItemID: r.ID,
			Code:           r.Code,
			Name:           r.Name,
			Slot:           r.Slot,
			Rarity:         r.Rarity,
			IconURL:        r.IconURL,
			AssetURL:       r.AssetURL,
		})
	}
	return briefs, nil
}
