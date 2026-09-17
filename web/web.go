// Package web 把前端靜態檔嵌進執行檔，讓伺服器在任何目錄下執行都能供應頁面，
// 不必依賴相對路徑找得到 web 目錄。
package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html style.css app.js games
var files embed.FS

// FS 回傳內嵌的前端檔案。
func FS() fs.FS { return files }
