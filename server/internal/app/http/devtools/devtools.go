// Package devtools 提供 /dev/* 路由组与相关中间件，用于开发期 / demo 期的
// 辅助端点（如 Story 7.5 POST /dev/grant-steps、Story 20.7 POST /dev/force-unlock-chest）。
//
// # 启用方式（OR 语义，任一成立即启用）
//
//   - 运行期：环境变量 BUILD_DEV=true（严格字面匹配，**不**接受 "1"/"yes"/"TRUE"）
//   - 编译期：build tag `-tags devtools`（生产二进制应 **不** 带此 tag）
//
// 生产构建推荐：`go build ./...`（无 tag）+ 运维 SOP 不设 BUILD_DEV，
// 这样 Register 不注册任何 /dev 路由，端点物理不可达。
//
// # 双闸门（防御纵深）
//
//  1. Register 在 IsEnabled()==false 时直接返回，不挂 /dev/* 路由组 → Gin 默认 NoRoute 返回文本 404
//  2. DevOnlyMiddleware 在 request 时再做一次 IsEnabled() 校验 → false 则返回 envelope 404（code=1003）
//     这为"挂了路由但运行期关闭 BUILD_DEV"（极边缘但实现成本为零）与"单独被测 middleware"场景兜底
//
// # 验证命令
//
//	go build ./...                  # 生产路径（forceDevEnabled=false）
//	go build -tags devtools ./...   # dev 路径（forceDevEnabled=true）
//	go test ./...                   # 默认跑 env-var 路径的所有 case
//	go test -tags devtools ./...    # 追加跑 build-tag 强制启用下的测试
//
// # 与业务模块的关系
//
// 本包只做 **框架**：Register + DevOnlyMiddleware + 示例 PingDevHandler。
// 业务 dev 端点（`/dev/grant-steps` / `/dev/force-unlock-chest` / `/dev/grant-cosmetic-batch`）
// 由各自业务 Epic 的 story 扩展 Register，**不**在本包内累积。
package devtools

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/logger"
	"github.com/huing/cat/server/internal/pkg/response"
)

// envBuildDev 是启用 dev 模式的运行期环境变量名。严格字面 "true" 才视为真。
const envBuildDev = "BUILD_DEV"

// devNotFoundCode 是 DevOnlyMiddleware 被拒请求返回的业务错误码，
// 对齐 V1接口设计 §3 的 `1003 资源不存在`。
//
// 选 1003（而非 403/401）是出于 OpSec 考虑：对外与"路径不存在"无差别，
// 不泄露"dev 端点存在但被拒"的信息。
const devNotFoundCode = 1003

// IsEnabled 返回 dev 模式是否启用。
//
// 两个触发源任一成立即为 true（OR 语义）：
//   - forceDevEnabled（编译期 `-tags devtools` 决定；见 buildtag_*.go）
//   - os.Getenv(envBuildDev) == "true"（运行期环境变量，严格字面匹配）
//
// 故意**不**缓存到包级 var：每次调用实时读 env，允许 DevOnlyMiddleware 在
// request 阶段响应 env 的运维热切换（极边缘场景，但实现开销为零）。
func IsEnabled() bool {
	return forceDevEnabled || os.Getenv(envBuildDev) == "true"
}

// Register 把 /dev/* 路由组挂到传入的 gin.Engine 上（仅在 dev 模式启用时）。
//
// 未启用时完全透明：不注册路由、不打印日志；调用方拿到的 engine 与不调用本函数等价。
//
// 启用时的副作用：
//  1. 输出一条 WARN：`DEV MODE ENABLED - DO NOT USE IN PRODUCTION`
//     （携带 build_tag_devtools / env_build_dev 字段，便于日志排障溯源触发源）
//  2. 创建 /dev 路由组并挂 DevOnlyMiddleware
//  3. 注册 GET /dev/ping-dev → PingDevHandler
//
// Register **不是**幂等的：在同一 engine 上重复调用会让 Gin panic（路由重复注册）。
// 但调用方只有 bootstrap.NewRouter() 一处，天然只调一次，不再额外防御。
func Register(r *gin.Engine) {
	if !IsEnabled() {
		return
	}
	slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION",
		slog.Bool("build_tag_devtools", forceDevEnabled),
		slog.String("env_build_dev", os.Getenv(envBuildDev)),
	)
	g := r.Group("/dev")
	g.Use(DevOnlyMiddleware())
	g.GET("/ping-dev", PingDevHandler)
}

// DevOnlyMiddleware 是 /dev/* 路由组的前置中间件，提供 **请求时**的第二道闸门。
//
// 行为：
//   - IsEnabled()==true：c.Next() 放行
//   - IsEnabled()==false：
//     1. 取 ctx 里已被 Logging 中间件注入的 child logger（带 request_id）
//     2. 打一条 WARN：`dev_only middleware rejected request`
//     携带 api_path / method / client_ip（**不**记 user_id：dev 端点不走 auth；
//     **不**记 request body：可能过大，日志放大）
//     3. 返回 envelope 404（code=1003 "资源不存在"）+ c.Abort()
//
// 日志级别为 WARN 而非 ERROR：被拒是**预期**防御路径，不是错误（ERROR 会污染告警）。
func DevOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsEnabled() {
			c.Next()
			return
		}
		reqLogger := logger.FromContext(c.Request.Context())
		reqLogger.WarnContext(c.Request.Context(), "dev_only middleware rejected request",
			slog.String("api_path", c.FullPath()),
			slog.String("method", c.Request.Method),
			slog.String("client_ip", c.ClientIP()),
		)
		response.Error(c, http.StatusNotFound, devNotFoundCode, "资源不存在")
		c.Abort()
	}
}

// PingDevHandler 是 /dev/ping-dev 的示例端点，用于验证 dev tools 框架本身可用。
//
// 这是 Story 1.6 提供的 **唯一** dev 端点。业务 dev 端点（步数 / 宝箱 / 装扮）
// 由后续 story 引入。
//
// 响应：{code:0, message:"ok", data:{"mode":"dev"}, requestId}
func PingDevHandler(c *gin.Context) {
	response.Success(c, gin.H{"mode": "dev"}, "ok")
}
