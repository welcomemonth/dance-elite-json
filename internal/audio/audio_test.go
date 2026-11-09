package audio

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
}

func TestNewManager_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audio")
	if _, err := NewManager(dir); err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("目录应被自动创建: %v", err)
	}
}

func TestScan_FiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "U13-A2-张三.mp3")
	writeFile(t, dir, "U13-A10-李四.wav")
	writeFile(t, dir, "U13-A1-王五.mp3")
	writeFile(t, dir, "ignore.txt")
	writeFile(t, dir, "ignore.jpg")
	writeFile(t, dir, "U13-A1-王五.MP3") // 大小写不敏感，应被识别
	// 子目录应被忽略（即使名字带 .mp3）
	if err := os.Mkdir(filepath.Join(dir, "sub.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}

	list := m.List()
	// 3 个 mp3/wav + 1 个 .MP3 = 4，忽略 txt/jpg/子目录
	if len(list) != 4 {
		t.Fatalf("len = %d, want 4", len(list))
	}
	// 自然排序：A1(小写 mp3 与大写 MP3 同 Name，按字符串序并列) 在 A2 前，A10 最后
	want := []string{"U13-A1-王五", "U13-A1-王五", "U13-A2-张三", "U13-A10-李四"}
	for i, a := range list {
		if a.Name != want[i] {
			t.Errorf("list[%d].Name = %q, want %q", i, a.Name, want[i])
		}
	}
}

func TestScan_EmptyDir(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}
	if list := m.List(); len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

func TestList_NotEmptyForEmptyDir(t *testing.T) {
	// 空目录也应返回非 nil 切片（保证 JSON 序列化为 [] 而非 null）
	m, _ := NewManager(t.TempDir())
	if m.List() == nil {
		t.Error("空目录下 List() 不应为 nil")
	}
}

func TestBuildURL_RoundTrip(t *testing.T) {
	filename := "U13-A9-刘芊惠-《白瓷旋香》.mp3"
	u := buildURL(filename)
	if !strings.HasPrefix(u, "/api/contest/audio/") {
		t.Fatalf("URL 前缀错误: %q", u)
	}
	got, err := url.PathUnescape(strings.TrimPrefix(u, "/api/contest/audio/"))
	if err != nil {
		t.Fatalf("PathUnescape 失败: %v", err)
	}
	if got != filename {
		t.Errorf("round-trip = %q, want %q", got, filename)
	}
}

func TestPath_Safety(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.mp3")
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}

	if _, ok := m.Path("a.mp3"); !ok {
		t.Error("已扫描文件应命中")
	}
	if _, ok := m.Path("b.mp3"); ok {
		t.Error("未扫描文件不应命中")
	}
	if _, ok := m.Path("../config.json"); ok {
		t.Error("路径穿越应被拒绝")
	}
	if _, ok := m.Path("/etc/passwd"); ok {
		t.Error("绝对路径应被拒绝")
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"U13-A2", "U13-A10", true},
		{"U13-A9", "U13-A10", true},
		{"A1", "A2", true},
		{"A10", "A2", false},
		{"2", "10", true},
		{"abc", "abd", true},
		{"abc", "abc", false},
		{"A1", "AB", true}, // 数字排在非数字之前
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
