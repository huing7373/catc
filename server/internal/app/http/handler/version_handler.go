package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/buildinfo"
	"github.com/huing/cat/server/internal/pkg/response"
)

// VersionResponse 是 /version 的 data 字段结构。
// JSON tag 对齐 V1接口设计 §2.4 小驼峰约定（builtAt 而非 built_at）。
type VersionResponse struct {
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

// VersionHandler 返回当前服务的 git commit 与构建时间。
//
// 两个字段的值在编译期通过 -ldflags -X 注入 buildinfo 包；
// 未注入时（例如直接 go run 或 test binary）默认返回 "unknown"。
//
// 响应严格对齐统一 envelope：
//
//	{code:0, message:"ok", data:{commit, builtAt}, requestId}
func VersionHandler(c *gin.Context) {
	response.Success(c, VersionResponse{
		Commit:  buildinfo.Commit,
		BuiltAt: buildinfo.BuiltAt,
	}, "ok")
}
