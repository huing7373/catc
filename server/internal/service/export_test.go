// Package service test-only export hooks.
//
// 本文件**仅在测试构建时**编译进 service 包（_test.go 后缀语义），
// 给 service_test 包暴露内部钩子，避免在生产 API 上增加只服务于测试的 setter。
//
// 当前唯一钩子：
//
//   - SetChestServiceNowFn — 覆盖 chestServiceImpl.nowFn 字段。20.9 r2 引入用于
//     Story 20.9 集成测试 AC13 unlock_at=-1ms 边界 case：必须用 fixed clock 才能
//     精确验证 service 内 `chest.UnlockAt <= s.nowFn()` 的边界（"now - 1ms <= now"
//     必须为真）。**不**走 wall clock —— busy CI runner 上 service 内 s.nowFn()
//     和 test 内 time.Now() 的间隔 >> 1ms 时，即使 service 把 <= 错改成 < 也
//     仍然能误测试通过（lesson：docs/lessons/2026-05-15-fixed-clock-for-boundary-tests.md）。
//
// **设计选择 vs 在生产 API 加 WithNowFn option**：
//
//   - 加生产 option：所有 caller 需识别 nowFn 选项，徒增 NewChestService 签名复杂度；
//   - export_test.go 钩子：production binary 不携带本文件代码（_test.go 仅 go test 时编入），
//     纯测试用途零生产副作用 —— 符合 Go 标准库 io / encoding/json 等的内部测试钩子模式。
//
// 调用方约束：
//
//   - **必须**在 service_test 包内通过 service.SetChestServiceNowFn 调用（导出大写头）；
//   - 入参 svc 必须是 NewChestService 返回的 *chestServiceImpl 实例 —— 其他实装会 panic；
//   - fn 必须返回 UTC 时间（与生产路径默认 `time.Now().UTC()` 一致；避免单测因时区差异 flaky）。
package service

import "time"

// SetChestServiceNowFn 覆盖 chestServiceImpl.nowFn 字段（仅测试用）。
//
// 不在生产 surface（本文件 _test.go 后缀仅 go test 编译）。
//
// 如果 svc 不是 *chestServiceImpl 类型，panic —— 这是 caller bug，
// 不应在生产路径触发。
func SetChestServiceNowFn(svc ChestService, fn func() time.Time) {
	impl, ok := svc.(*chestServiceImpl)
	if !ok {
		panic("SetChestServiceNowFn: svc is not *chestServiceImpl")
	}
	impl.nowFn = fn
}
