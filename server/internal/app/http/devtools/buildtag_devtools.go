//go:build devtools

package devtools

// forceDevEnabled 在 build tag `devtools` 启用时为 true（强制开启 dev 模式），
// 无须设置 BUILD_DEV 环境变量。
//
// 对应文件：buildtag_normal.go（//go:build !devtools，值为 false）。
const forceDevEnabled = true
