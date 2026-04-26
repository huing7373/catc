package db

import "time"

// shortTimeout 是单元测试用的短 ctx timeout，避免 unreachable host 测试拖慢套件。
//
// 选 2s 是平衡：太短（< 500ms）在慢机 / CI 上可能在 dial 之前就 cancel → 测试不稳定；
// 太长（> 5s）会让 -count=1 整套测试每次多 5s。
func shortTimeout() time.Duration {
	return 2 * time.Second
}
