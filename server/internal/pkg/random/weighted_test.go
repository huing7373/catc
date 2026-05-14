package random_test

import (
	stderrors "errors"
	"math"
	mathrand "math/rand"
	"testing"

	"github.com/huing/cat/server/internal/pkg/random"
)

// Story 20.6 — WeightedPicker 单测（≥3 case）
//
// 验证：
//   1. SingleItem: 1 个元素 → 总是返回 index 0
//   2. MultipleItems_DistributionWithDeterministicSeed: 3 个元素权重 10/30/60
//      → 跑 N 次确定性 seed → 分布大致符合 10%/30%/60%
//   3. EmptyItems_ReturnsError: 空 slice → ErrEmptyItems
//   4. ZeroTotalWeight_ReturnsError: 全 0 权重 → ErrZeroTotalWeight

// TestWeightedPicker_Pick_SingleItem: 1 个元素 → 返 index 0。
func TestWeightedPicker_Pick_SingleItem(t *testing.T) {
	// mathrand.New 自带 Read 方法实现 io.Reader
	picker := random.NewCryptoWeightedPicker(mathrand.New(mathrand.NewSource(42)))
	idx, err := picker.Pick([]random.WeightedItem{{Weight: 100}})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0 (single item)", idx)
	}
}

// TestWeightedPicker_Pick_MultipleItems_DistributionWithDeterministicSeed:
// 3 个元素权重 10/30/60；跑 N=10000 次确定性 seed → 分布应大致符合 10%/30%/60%
// （±5% 容差防脆弱）。
func TestWeightedPicker_Pick_MultipleItems_DistributionWithDeterministicSeed(t *testing.T) {
	const N = 10000
	picker := random.NewCryptoWeightedPicker(mathrand.New(mathrand.NewSource(123)))
	items := []random.WeightedItem{
		{Weight: 10},
		{Weight: 30},
		{Weight: 60},
	}

	counts := [3]int{}
	for i := 0; i < N; i++ {
		idx, err := picker.Pick(items)
		if err != nil {
			t.Fatalf("Pick #%d: %v", i, err)
		}
		if idx < 0 || idx >= 3 {
			t.Fatalf("Pick #%d: idx = %d out of range", i, idx)
		}
		counts[idx]++
	}

	expected := []float64{0.10, 0.30, 0.60}
	for i, exp := range expected {
		actual := float64(counts[i]) / float64(N)
		if math.Abs(actual-exp) > 0.05 {
			t.Errorf("items[%d] distribution = %.4f (count=%d), want ~%.2f (±0.05 tol)",
				i, actual, counts[i], exp)
		}
	}
}

// TestWeightedPicker_Pick_EmptyItems_ReturnsError: 空 slice → ErrEmptyItems。
func TestWeightedPicker_Pick_EmptyItems_ReturnsError(t *testing.T) {
	picker := random.NewCryptoWeightedPicker(mathrand.New(mathrand.NewSource(1)))
	idx, err := picker.Pick([]random.WeightedItem{})
	if !stderrors.Is(err, random.ErrEmptyItems) {
		t.Errorf("err = %v, want ErrEmptyItems", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0 on error", idx)
	}
}

// TestWeightedPicker_Pick_ZeroTotalWeight_ReturnsError: 全 weight=0 → ErrZeroTotalWeight。
func TestWeightedPicker_Pick_ZeroTotalWeight_ReturnsError(t *testing.T) {
	picker := random.NewCryptoWeightedPicker(mathrand.New(mathrand.NewSource(1)))
	idx, err := picker.Pick([]random.WeightedItem{{Weight: 0}, {Weight: 0}, {Weight: 0}})
	if !stderrors.Is(err, random.ErrZeroTotalWeight) {
		t.Errorf("err = %v, want ErrZeroTotalWeight", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0 on error", idx)
	}
}
