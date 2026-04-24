//go:build !devtools

package devtools

// forceDevEnabled 是编译期由 build tag 决定的 dev 模式强制开关。
//
// 此文件仅在 **未** 传 `-tags devtools` 时参与编译（默认生产构建），
// 常量值为 false —— dev 模式只能靠运行期环境变量 BUILD_DEV=true 启用。
//
// 对应文件：buildtag_devtools.go（//go:build devtools，值为 true）。
//
// 两文件 build tag 必须严格互补：少一个 "!" 或多一个 tag 都会在任一路径下
// 让两个 const 同时定义，编译期报 `forceDevEnabled redeclared`。
// 验证命令：
//
//	go build ./...                  # 本文件生效（forceDevEnabled=false）
//	go build -tags devtools ./...   # 对应文件生效（forceDevEnabled=true）
const forceDevEnabled = false
