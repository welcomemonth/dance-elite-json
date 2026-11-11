package storage

import "strings"

// PublicURL 把数据库相对路径映射为对外访问前缀：
//
//	uploads/avatars/{player_no}/x → /static/avatars/{player_no}/x
//	uploads/{event_id}/{player_no}/x → /static/materials/{event_id}/{player_no}/x
//
// 已是绝对 URL 或未知前缀时原样返回，便于未来切换 CDN。
func PublicURL(relPath string) string {
	if relPath == "" {
		return ""
	}
	if rest, ok := strings.CutPrefix(relPath, relPrefix+"avatars/"); ok {
		return "/static/avatars/" + rest
	}
	if rest, ok := strings.CutPrefix(relPath, relPrefix); ok {
		return "/static/materials/" + rest
	}
	return relPath
}
