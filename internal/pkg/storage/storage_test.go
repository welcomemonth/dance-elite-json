package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestStageAndCommitMaterial(t *testing.T) {
	s := newTestStore(t)
	f, err := s.StageMaterial(12, "a8K2mP9x", KindPhoto, ".jpg", strings.NewReader("photo-bytes"))
	if err != nil {
		t.Fatalf("StageMaterial: %v", err)
	}
	if !strings.HasPrefix(f.RelPath, "uploads/12/a8K2mP9x/photo_") || !strings.HasSuffix(f.RelPath, ".jpg") {
		t.Fatalf("rel path = %q", f.RelPath)
	}
	// 提交前临时文件存在、最终文件不存在。
	if _, err := os.Stat(f.TempPath); err != nil {
		t.Fatalf("temp file missing: %v", err)
	}
	if _, err := os.Stat(s.abs(f.RelPath)); !os.IsNotExist(err) {
		t.Fatalf("final file exists before commit: %v", err)
	}

	if err := s.Commit(f); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	data, err := os.ReadFile(s.abs(f.RelPath))
	if err != nil {
		t.Fatalf("read committed file: %v", err)
	}
	if string(data) != "photo-bytes" {
		t.Fatalf("content = %q", data)
	}
	if _, err := os.Stat(f.TempPath); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists after commit: %v", err)
	}
}

func TestStageAvatar(t *testing.T) {
	s := newTestStore(t)
	f, err := s.StageAvatar("a8K2mP9x", "jpg", bytes.NewReader([]byte("avatar")))
	if err != nil {
		t.Fatalf("StageAvatar: %v", err)
	}
	if err := s.Commit(f); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !strings.HasPrefix(f.RelPath, "uploads/avatars/a8K2mP9x/avatar_") {
		t.Fatalf("rel path = %q", f.RelPath)
	}
	if _, err := os.Stat(s.abs(f.RelPath)); err != nil {
		t.Fatalf("avatar file missing: %v", err)
	}
}

func TestRemoveRejectsTraversalAndMissingIsOK(t *testing.T) {
	s := newTestStore(t)
	if err := s.Remove("uploads/../secret.txt"); err == nil {
		t.Fatal("Remove traversal path: want error")
	}
	if err := s.Remove("../secret.txt"); err == nil {
		t.Fatal("Remove relative traversal: want error")
	}
	if err := s.Remove("uploads/12/none/none.jpg"); err != nil {
		t.Fatalf("Remove missing file: %v", err)
	}

	f, err := s.StageMaterial(12, "abc12345", KindAudio, "mp3", strings.NewReader("audio"))
	if err != nil {
		t.Fatalf("StageMaterial: %v", err)
	}
	if err := s.Commit(f); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := s.Remove(f.RelPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(s.abs(f.RelPath)); !os.IsNotExist(err) {
		t.Fatalf("file still exists after Remove: %v", err)
	}
}

func TestStageRejectsUnsafeSegment(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.StageMaterial(12, "../evil", KindPhoto, "jpg", strings.NewReader("x")); err == nil {
		t.Fatal("unsafe player_no accepted")
	}
	if _, err := s.StageMaterial(0, "abc12345", KindPhoto, "jpg", strings.NewReader("x")); err == nil {
		t.Fatal("invalid event id accepted")
	}
}

func TestStoreRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "uploads")
	s, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Root() != root {
		t.Fatalf("root = %q, want %q", s.Root(), root)
	}
	if _, err := New(""); err == nil {
		t.Fatal("New with empty root: want error")
	}
}
