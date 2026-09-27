package web

import "embed"

// FS 持有前端静态资源，编译期通过 go:embed 嵌入二进制。
// 注意：embed 指令必须与被嵌入资源位于同一目录（不允许使用 .. 路径）。
//
//go:embed index.html style.css app.js cover.jpg
var FS embed.FS
