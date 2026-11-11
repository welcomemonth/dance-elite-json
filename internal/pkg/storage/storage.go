// Package storage 提供参赛资料媒体的本地磁盘存储（开发计划 T13，R8 定稿）。
//
// 目录约定（数据库只存相对路径，始终以 uploads/ 开头）：
//
//	uploads/{event_id}/{player_no}/{kind}_{随机8字节hex}.{ext}   证件照/音频
//	uploads/avatars/{player_no}/avatar_{随机8字节hex}.jpg        证件照压缩副本
//
// 上传流程：写入 uploads/tmp 下的临时文件 → 校验由 service 层完成 →
// Commit 原子 rename 到最终路径 → service 更新数据库。服务端随机文件名
// 不使用客户端原始名，避免覆盖与路径穿越；uploads/ 不入库、不进镜像。
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// 媒体种类（用于生成文件名前缀）。
const (
	KindPhoto = "photo"
	KindAudio = "audio"
)

// relPrefix 是数据库中媒体相对路径的固定前缀。
const relPrefix = "uploads/"

// segmentPattern 校验路径段（player_no / event_id）只含安全字符。
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Store 是 uploads/ 根目录的本地文件存储。
type Store struct {
	root string
	tmp  string
}

// New 创建存储并确保临时目录存在。root 为物理目录（配置 UPLOAD_DIR，
// 默认 uploads），数据库中的相对路径不受 root 影响，始终为 uploads/...。
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("upload dir is empty")
	}
	tmp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, fmt.Errorf("create upload temp dir %q: %w", tmp, err)
	}
	return &Store{root: root, tmp: tmp}, nil
}

// StagedFile 是已写入临时文件、尚未提交到最终路径的上传文件。
type StagedFile struct {
	TempPath string // 物理临时路径
	RelPath  string // 数据库相对路径，如 uploads/12/abc123/photo_1a2b3c4d.jpg
}

// StageMaterial 把 src 写入临时文件，并生成证件照/音频的最终相对路径。
// 调用方须在业务校验通过后调用 Commit 才会出现在最终目录。
func (s *Store) StageMaterial(eventID int64, playerNo, kind, ext string, src io.Reader) (*StagedFile, error) {
	if kind != KindPhoto && kind != KindAudio {
		return nil, fmt.Errorf("unsupported material kind %q", kind)
	}
	if eventID <= 0 {
		return nil, fmt.Errorf("invalid event id %d", eventID)
	}
	if err := validateSegment(playerNo); err != nil {
		return nil, err
	}
	dir := fmt.Sprintf("%s%d/%s", relPrefix, eventID, playerNo)
	return s.stage(dir, fmt.Sprintf("%s_%s.%s", kind, randomHex(8), trimDot(ext)), src)
}

// StageAvatar 把压缩后的头像写入临时文件，最终路径在 avatars/{player_no}/ 下。
func (s *Store) StageAvatar(playerNo, ext string, src io.Reader) (*StagedFile, error) {
	if err := validateSegment(playerNo); err != nil {
		return nil, err
	}
	dir := relPrefix + "avatars/" + playerNo
	return s.stage(dir, fmt.Sprintf("avatar_%s.%s", randomHex(8), trimDot(ext)), src)
}

// stage 写入临时文件并生成最终相对路径（Commit 前不对最终目录可见）。
func (s *Store) stage(dir, name string, src io.Reader) (*StagedFile, error) {
	f, err := os.CreateTemp(s.tmp, "upload-*")
	if err != nil {
		return nil, fmt.Errorf("create temp upload file: %w", err)
	}
	tmpPath := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}
	if _, err := io.Copy(f, src); err != nil {
		cleanup()
		return nil, fmt.Errorf("write temp upload file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp upload file: %w", err)
	}
	return &StagedFile{TempPath: tmpPath, RelPath: path.Join(dir, name)}, nil
}

// Commit 创建最终目录并把临时文件原子 rename 到最终路径。
// 同一文件系统内 rename 成功即对外可见，不存在半写状态。
func (s *Store) Commit(f *StagedFile) error {
	abs := s.abs(f.RelPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("create material dir for %q: %w", f.RelPath, err)
	}
	if err := os.Rename(f.TempPath, abs); err != nil {
		return fmt.Errorf("commit upload %q: %w", f.RelPath, err)
	}
	return nil
}

// Discard 删除尚未提交的临时文件（已提交时为空操作）。
func (s *Store) Discard(f *StagedFile) {
	if f == nil || f.TempPath == "" {
		return
	}
	_ = os.Remove(f.TempPath)
}

// Remove 删除相对路径对应的文件（旧资料清理）。只接受 uploads/ 下的路径，
// 路径穿越或越界路径直接拒绝；文件不存在视为成功。
func (s *Store) Remove(relPath string) error {
	if relPath == "" {
		return nil
	}
	clean := path.Clean(relPath)
	if clean == "uploads" || !strings.HasPrefix(clean, relPrefix) {
		return fmt.Errorf("refuse to remove path outside uploads: %q", relPath)
	}
	if err := os.Remove(s.abs(clean)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove upload %q: %w", clean, err)
	}
	return nil
}

// Root 返回存储的物理根目录。
func (s *Store) Root() string { return s.root }

// abs 把相对路径转为物理路径；前缀已由调用方校验。
func (s *Store) abs(relPath string) string {
	return filepath.Join(s.root, filepath.FromSlash(strings.TrimPrefix(relPath, relPrefix)))
}

// validateSegment 拒绝空值与路径分隔符等特殊字符，防止路径穿越。
func validateSegment(segment string) error {
	if !segmentPattern.MatchString(segment) {
		return fmt.Errorf("invalid path segment %q", segment)
	}
	return nil
}

// trimDot 去掉扩展名可能带的前导点。
func trimDot(ext string) string { return strings.TrimPrefix(ext, ".") }

// randomHex 返回 n 字节 crypto/rand 的十六进制字符串（用于服务端文件名）。
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 在受支持平台不会返回错误；保底回退避免 panic。
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(buf)
}
