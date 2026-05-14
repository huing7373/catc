// Package random 提供 server 业务用的随机抽样工具集（Story 20.6 引入）。
//
// 设计目标（V1 §7.2.5g + epics.md §20.9 行 4403 警示）：
//   - 不依赖 `math/rand` 全局源（默认 seed=1 序列可预测，开箱抽奖存在作弊风险）
//   - 通过 io.Reader interface 注入随机源 —— 生产用 `crypto/rand.Reader`，
//     单测注入确定性 `mathrand.New(mathrand.NewSource(seed))` 验证分布
//   - 单一职责：本包仅"抽样"，不做业务概念封装（权重 / 物品 ID / 抽奖语义）
//
// 当前唯一公开 API：WeightedPicker.Pick(items)。
//
// **范围红线**：本包不依赖业务 domain 类型（如 mysql.CosmeticItem）；接受 generic
// `[]WeightedItem` slice，让 service 层负责把 cosmetic_items.drop_weight 翻译为
// WeightedItem.Weight。这样 random 包对业务变更天然解耦（Epic 23 合成 / Epic 32
// 同步抽奖路径都能复用本包）。
package random

import (
	"encoding/binary"
	"errors"
	"io"
)

// ErrEmptyItems: WeightedPicker.Pick 入参为空 slice 时返回。
// 调用方应在调用前校验非空 + 翻译为业务错（如 1009 数据完整性异常）。
var ErrEmptyItems = errors.New("random: empty items")

// ErrZeroTotalWeight: WeightedPicker.Pick 入参权重总和为 0 时返回（理论不应发生 ——
// service 层已过滤 is_enabled=1 + drop_weight > 0 的行；兜底防御）。
var ErrZeroTotalWeight = errors.New("random: total weight is zero")

// WeightedItem 是 Pick 的入参元素 —— 仅含权重字段，业务 id / metadata 由 caller 保留。
//
// **关键设计**：本 type 不含 ID / Name 等业务字段；调用方持有原始 slice
// （如 []CosmeticItem），调用 Pick(items) 取 selectedIndex 后回到原 slice 拿
// 真实物品对象。这让 random 包对 domain 类型透明。
type WeightedItem struct {
	Weight uint64
}

// WeightedPicker 是加权抽样的对外接口。
//
// 抽样语义：
//   - 累加 items[i].Weight 得 total
//   - 从 reader 取 8 字节 → uint64 → mod total → 二分查找命中的区间 → 返该区间的 index
//
// 错误：
//   - ErrEmptyItems: len(items) == 0
//   - ErrZeroTotalWeight: 累加 total == 0（所有 weight=0）
//   - reader 读失败 → 透传 reader.Read error
type WeightedPicker interface {
	// Pick 从 items 中按权重抽样一个 index。
	//
	// **不**修改 items slice；调用方可并发调（reader 不并发安全则需要外层加锁，
	// crypto/rand.Reader 内部用 mutex 是并发安全的）。
	Pick(items []WeightedItem) (int, error)
}

// cryptoWeightedPicker 是 WeightedPicker 的默认实装，注入 io.Reader 取随机字节。
//
// 命名虽然带 "crypto"，但实际可注入任何 io.Reader —— 单测注入 mathrand 走确定性
// 序列；生产注入 crypto/rand.Reader 走 OS 层加密随机源。
//
// 命名沿用"crypto"是因为生产路径走 crypto/rand —— 但作为类型名仅是"实装类标识"，
// 不强制随机源类型。
type cryptoWeightedPicker struct {
	reader io.Reader
}

// NewCryptoWeightedPicker 构造 WeightedPicker，注入随机源 reader。
//
// 调用方：
//   - 生产：random.NewCryptoWeightedPicker(crypto_rand.Reader)
//   - 单测：random.NewCryptoWeightedPicker(mathrand.New(mathrand.NewSource(seed)))
//     （需要 wrap mathrand.Rand 实现 io.Reader —— mathrand.Rand 自带 Read 方法）
//
// reader == nil → 调用 Pick 时 panic（caller bug；构造期不 panic，让单测可注入 nil 验证
// 边界 —— 但实际场景几乎不会有 caller 传 nil）。
func NewCryptoWeightedPicker(reader io.Reader) WeightedPicker {
	return &cryptoWeightedPicker{reader: reader}
}

// Pick 实装：
//
//  1. len(items)==0 → ErrEmptyItems
//  2. 累加 total = sum(items[i].Weight)
//  3. total==0 → ErrZeroTotalWeight
//  4. 从 reader 读 8 字节 → binary.BigEndian.Uint64 → mod total → r ∈ [0, total)
//  5. 走 cumulative weight 数组找 r 命中的第一个 index（O(n) 线性扫描，items 数量
//     在节点 7 阶段约 15-20，比构造前缀和数组 + 二分查找更简单且性能等价）
func (p *cryptoWeightedPicker) Pick(items []WeightedItem) (int, error) {
	if len(items) == 0 {
		return 0, ErrEmptyItems
	}
	var total uint64
	for _, it := range items {
		total += it.Weight
	}
	if total == 0 {
		return 0, ErrZeroTotalWeight
	}

	var buf [8]byte
	if _, err := io.ReadFull(p.reader, buf[:]); err != nil {
		return 0, err
	}
	r := binary.BigEndian.Uint64(buf[:]) % total

	// 线性扫描找 r 命中的区间。
	// items[0] 区间: [0, items[0].Weight)
	// items[1] 区间: [items[0].Weight, items[0].Weight + items[1].Weight)
	// ...
	var cum uint64
	for i, it := range items {
		cum += it.Weight
		if r < cum {
			return i, nil
		}
	}
	// 理论不可达（r < total 必然在某个 cum < total 之前命中）；兜底返回最后一项。
	return len(items) - 1, nil
}
