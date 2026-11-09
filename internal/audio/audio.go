package audio

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

// AudioInfo 表示一个音频文件，用于页面展示与播放。
type AudioInfo struct {
	Name string `json:"name"` // 去扩展名后的文件名，用于页面展示
	URL  string `json:"url"`  // 音频播放地址，含扩展名并已做 URL 段转义
}

// Manager 在服务启动时扫描音频目录，并将结果缓存到内存。
// 运行期间不重新扫描：新增/删除/改名音频需重启服务后生效。
type Manager struct {
	dir   string            // 音频目录绝对路径
	list  []AudioInfo       // 已自然排序的音频列表
	paths map[string]string // 文件名(含扩展名) -> 磁盘绝对路径，作为访问白名单
}

// NewManager 创建音频管理器并立即扫描指定目录。
// 目录不存在时自动创建；目录存在但无音频文件时返回空列表而不报错。
func NewManager(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		dir:   dir,
		paths: make(map[string]string),
	}
	if err := m.scan(); err != nil {
		return nil, err
	}
	return m, nil
}

// scan 读取音频目录，过滤 .mp3/.wav，构建列表并按文件名自然排序。
func (m *Manager) scan() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}

	list := make([]AudioInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != ".mp3" && ext != ".wav" {
			continue
		}
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		logger.Info("文件名", name)
		m.paths[filename] = filepath.Join(m.dir, filename)
		list = append(list, AudioInfo{
			Name: name,
			URL:  buildURL(filename),
		})
	}

	sort.Slice(list, func(i, j int) bool {
		return naturalLess(list[i].Name, list[j].Name)
	})
	m.list = list
	return nil
}

// List 返回已自然排序的音频列表（调用方只读，勿修改）。
// 空目录时返回非 nil 的空切片，保证 JSON 序列化为 [] 而非 null。
func (m *Manager) List() []AudioInfo {
	return m.list
}

// Path 返回文件名对应的磁盘绝对路径。
// 仅当文件名在启动时扫描到的白名单内才返回 ok=true，以此杜绝路径穿越。
func (m *Manager) Path(filename string) (string, bool) {
	p, ok := m.paths[filename]
	return p, ok
}

// buildURL 生成音频播放地址，文件名做 URL 段转义以兼容中文与特殊字符。
func buildURL(filename string) string {
	return "/api/contest/audio/" + url.PathEscape(filename)
}
