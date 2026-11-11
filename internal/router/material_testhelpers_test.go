package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/storage"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newMaterialTestEngine 装配含资料上传路由与静态文件访问的测试引擎，
// 返回签名器、DB 与上传根目录。
func newMaterialTestEngine(t *testing.T) (*gin.Engine, *token.Signer, *store.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Seed(context.Background(), store.SeedOptions{
		AdminUsername: "admin", AdminPassword: "Init@12345",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stores := store.New(db)
	signer := token.NewSigner("test-secret", time.Hour)
	players := service.NewPlayerService(stores, "test-hmac-key")
	root := t.TempDir()
	files, err := storage.New(root)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return New(Deps{
		Stores:        stores,
		AdminAuth:     service.NewAdminAuthService(stores, signer),
		Seasons:       service.NewSeasonService(stores),
		Events:        service.NewEventService(stores),
		PublicEvents:  service.NewPublicEventService(stores),
		Registrations: service.NewRegistrationService(stores, players),
		Materials:     service.NewMaterialService(stores, files),
		Tokens:        signer,
		UploadDir:     root,
	}), signer, db, root
}

// insertActiveRegistration 直接插入一条 active 报名（资料接口不依赖下单流程），
// 返回报名单 ID 与选手编号。
func insertActiveRegistration(t *testing.T, db *store.DB, eventID, userID int64) (int64, string) {
	t.Helper()
	ctx := context.Background()
	var groupID int64
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT id FROM event_groups WHERE event_id = ? AND enabled = 1 ORDER BY id LIMIT 1`, eventID).Scan(&groupID); err != nil {
		t.Fatalf("query group: %v", err)
	}
	playerNo := "a8K2mP9x"
	res, err := db.SQL().ExecContext(ctx,
		`INSERT INTO players (player_no, name) VALUES (?, ?)`, playerNo, "王小舞")
	if err != nil {
		t.Fatalf("insert player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	res, err = db.SQL().ExecContext(ctx,
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status)
		 VALUES (?, ?, ?, ?, 'classical', 'active')`, eventID, groupID, playerID, userID)
	if err != nil {
		t.Fatalf("insert registration: %v", err)
	}
	regID, _ := res.LastInsertId()
	return regID, playerNo
}

type uploadFile struct {
	name string
	data []byte
}

// putMultipart 发送 multipart PUT 请求。fields 只包含显式提交的文本字段。
func putMultipart(t *testing.T, engine *gin.Engine, target string,
	fields map[string]string, files map[string]uploadFile, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	for field, f := range files {
		part, err := w.CreateFormFile(field, f.name)
		if err != nil {
			t.Fatalf("create form file %s: %v", field, err)
		}
		if _, err := part.Write(f.data); err != nil {
			t.Fatalf("write form file %s: %v", field, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, target, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func testRouterJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// testRouterMP3 构造 n 个 MPEG1 Layer III 128kbps/44.1kHz 帧（约 26.12ms/帧）。
func testRouterMP3(n int) []byte {
	frame := make([]byte, 417)
	frame[0], frame[1], frame[2], frame[3] = 0xff, 0xfb, 0x90, 0x64
	out := make([]byte, 0, n*len(frame))
	for i := 0; i < n; i++ {
		out = append(out, frame...)
	}
	return out
}

// putJSON 发送 JSON PUT 请求。
func putJSON(t *testing.T, engine *gin.Engine, target string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, target, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// materialTestSetup 创建赛季/赛事/用户/active 报名，返回赛事、报名、用户与 token。
func materialTestSetup(t *testing.T, engine *gin.Engine, signer *token.Signer, db *store.DB) (int64, int64, int64, string) {
	t.Helper()
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "资料上传赛事")
	userID := mustInsertRouterUser(t, db, fmt.Sprintf("o-material-%d", time.Now().UnixNano()))
	regID, _ := insertActiveRegistration(t, db, eventID, userID)
	return eventID, regID, userID, userTokenFor(t, signer, userID)
}

func mustUserIDOfRegistration(t *testing.T, db *store.DB, regID int64) int64 {
	t.Helper()
	var userID int64
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT user_id FROM registrations WHERE id = ?`, regID).Scan(&userID); err != nil {
		t.Fatalf("query registration owner: %v", err)
	}
	return userID
}
