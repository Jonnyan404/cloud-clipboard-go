//go:build !embed
// +build !embed

// filepath: /Users/jonny/Documents/GitHub/cloud-clipboard-go/cloud-clip/lib/static_default.go
package lib

import "embed"

// embed_static_fs 默认为空的嵌入文件系统
// 当不使用 'embed' 构建标签时使用此实现
var embed_static_fs embed.FS
