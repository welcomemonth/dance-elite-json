package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaterialUpdateAuthAndParams(t *testing.T) {
	engine, signer, db, _ := newMaterialTestEngine(t)
	_, regID, _, userToken := materialTestSetup(t, engine, signer, db)
	target := fmt.Sprintf("/api/v1/registrations/%d/materials", regID)

	// 未登录 → 40101。
	w := putMultipart(t, engine, target, map[string]string{"program_name": "春闺梦"}, nil, nil)
	assertCode(t, w, 40101)

	// 非法报名 ID → 40001。
	w = putMultipart(t, engine, "/api/v1/registrations/abc/materials",
		map[string]string{"program_name": "春闺梦"},
		nil, map[string]string{"Authorization": "Bearer " + userToken})
	assertCode(t, w, 40001)
}

func TestMaterialUpdateSuccessWithPhotoAndStaticServing(t *testing.T) {
	engine, signer, db, _ := newMaterialTestEngine(t)
	eventID, regID, _, userToken := materialTestSetup(t, engine, signer, db)
	target := fmt.Sprintf("/api/v1/registrations/%d/materials", regID)
	photo := testRouterJPEG(t, 900, 600)

	w := putMultipart(t, engine, target,
		map[string]string{"program_name": "春闺梦", "contact_phone": "13812345678"},
		map[string]uploadFile{"photo": {name: "id_photo.jpg", data: photo}},
		map[string]string{"Authorization": "Bearer " + userToken})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}

	var raw struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertSnakeCaseKeys(t, "material response", raw.Data)
	materials, ok := raw.Data["materials"].(map[string]any)
	if !ok {
		t.Fatalf("materials = %v", raw.Data["materials"])
	}
	assertSnakeCaseKeys(t, "materials", materials)
	photoURL, _ := materials["photo_url"].(string)
	wantPrefix := fmt.Sprintf("/static/materials/%d/a8K2mP9x/photo_", eventID)
	if !strings.HasPrefix(photoURL, wantPrefix) {
		t.Fatalf("photo_url = %q, want prefix %q", photoURL, wantPrefix)
	}
	if materials["audio_url"] != nil {
		t.Fatalf("audio_url = %v, want null", materials["audio_url"])
	}
	status, _ := raw.Data["material_status"].(map[string]any)
	if status == nil {
		t.Fatal("material_status missing")
	}
	assertSnakeCaseKeys(t, "material_status", status)
	if status["total"].(float64) != 8 || status["completed"].(float64) != 3 {
		t.Fatalf("material_status = %v, want 3/8", status)
	}

	// 静态访问：无需登录即可取回上传的证件照。
	req := httptest.NewRequest(http.MethodGet, photoURL, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("static GET %s = %d", photoURL, rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !bytes.Equal(body, photo) {
		t.Fatalf("static file content differs: got %d bytes", len(body))
	}
}

func TestMaterialUpdateRejectsNonOwnerAndInactive(t *testing.T) {
	engine, signer, db, _ := newMaterialTestEngine(t)
	_, regID, _, _ := materialTestSetup(t, engine, signer, db)
	otherUser := mustInsertRouterUser(t, db, "o-material-other")
	otherToken := userTokenFor(t, signer, otherUser)
	target := fmt.Sprintf("/api/v1/registrations/%d/materials", regID)

	w := putMultipart(t, engine, target, map[string]string{"program_name": "春闺梦"},
		nil, map[string]string{"Authorization": "Bearer " + otherToken})
	assertCode(t, w, 40401)

	// 已取消报名 → 41009。
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE registrations SET status = 'canceled' WHERE id = ?`, regID); err != nil {
		t.Fatalf("cancel registration: %v", err)
	}
	ownerToken := userTokenFor(t, signer, mustUserIDOfRegistration(t, db, regID))
	w = putMultipart(t, engine, target, map[string]string{"program_name": "春闺梦"},
		nil, map[string]string{"Authorization": "Bearer " + ownerToken})
	assertCode(t, w, 41009)
}

func TestMaterialUpdateRejectsWrongFileType(t *testing.T) {
	engine, signer, db, _ := newMaterialTestEngine(t)
	_, regID, _, userToken := materialTestSetup(t, engine, signer, db)
	target := fmt.Sprintf("/api/v1/registrations/%d/materials", regID)

	w := putMultipart(t, engine, target, nil,
		map[string]uploadFile{"photo": {name: "bad.gif", data: []byte("GIF89a")}},
		map[string]string{"Authorization": "Bearer " + userToken})
	assertCode(t, w, 40001)
}
