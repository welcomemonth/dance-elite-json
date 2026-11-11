package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/storage"
)

// materialFixture 在报名测试夹具基础上追加本地文件存储与资料服务。
type materialFixture struct {
	*regFixture
	files *storage.Store
	root  string
	svc   *MaterialService
}

func newMaterialFixture(t *testing.T) *materialFixture {
	t.Helper()
	f := newRegFixture(t)
	root := t.TempDir()
	files, err := storage.New(root)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return &materialFixture{regFixture: f, files: files, root: root, svc: NewMaterialService(f.stores, files)}
}

// mustActiveRegistration 走 T9 提交报名后直接把报名单置为 active（模拟支付成功），
// 返回报名单 ID、赛事 ID、选手 ID 与 player_no。
func mustActiveRegistration(t *testing.T, f *materialFixture, userID int64, key string) (int64, int64, int64, string) {
	t.Helper()
	eventID := mustCreateRegisteringEvent(t, f.regFixture, "资料赛事-"+key, defaultRegGroups)
	resp, err := f.regSvc.Submit(context.Background(), userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, key))
	if err != nil {
		t.Fatalf("submit registration: %v", err)
	}
	if _, err := f.db.SQL().ExecContext(context.Background(),
		`UPDATE registrations SET status = 'active' WHERE id = ?`, resp.RegistrationID); err != nil {
		t.Fatalf("activate registration: %v", err)
	}
	var playerID int64
	var playerNo string
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT p.id, p.player_no FROM players p JOIN registrations r ON r.player_id = p.id WHERE r.id = ?`,
		resp.RegistrationID).Scan(&playerID, &playerNo); err != nil {
		t.Fatalf("query player: %v", err)
	}
	return resp.RegistrationID, eventID, playerID, playerNo
}

// abs 把数据库相对路径转换为测试存储根下的物理路径。
func (f *materialFixture) abs(relPath string) string {
	return filepath.Join(f.root, filepath.FromSlash(strings.TrimPrefix(relPath, "uploads/")))
}

type readSeekCloser struct{ *bytes.Reader }

func (readSeekCloser) Close() error { return nil }

func testMaterialFile(name string, data []byte) *MaterialFile {
	return &MaterialFile{Filename: name, Size: int64(len(data)), Content: readSeekCloser{bytes.NewReader(data)}}
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 120, B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// pngBytes 生成一张 300x200 PNG（用于验证 png 扩展名与内容一致性）。
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, color.RGBA{G: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// testMP3 构造 n 个 MPEG1 Layer III 128kbps/44.1kHz 帧（约 26.12ms/帧）。
func testMP3(n int) []byte {
	frame := make([]byte, 417)
	frame[0], frame[1], frame[2], frame[3] = 0xff, 0xfb, 0x90, 0x64
	out := make([]byte, 0, n*len(frame))
	for i := 0; i < n; i++ {
		out = append(out, frame...)
	}
	return out
}

func requireBizCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want biz code %d", want)
	}
	var biz *httpx.BizError
	if !errors.As(err, &biz) {
		t.Fatalf("err = %v, want BizError %d", err, want)
	}
	if biz.Code != want {
		t.Fatalf("code = %d (%s), want %d", biz.Code, biz.Message, want)
	}
}
