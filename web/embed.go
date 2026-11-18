// Package web 把构建好的前端嵌进二进制。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Languages 是支持的语言清单（src/locales/languages.json）：界面、通知、聊天机器人共用这一份。
//
//go:embed src/locales/languages.json
var Languages []byte

// FS 返回构建好的前端；还没构建（没有 index.html）时返回 nil。
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
