package router

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestAdminMissingMaterialsHTTP 覆盖资料催收名单与标记已联系接口。
func TestAdminMissingMaterialsHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "资料催收接口", 10)
	userID := mustInsertRouterUser(t, db, "o-admin-material")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpMat01", "ORD-MAT-1", "paid", "miniapp")

	// 未登录 → 40101；cs 只读可用。
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/materials/missing", "", nil), 40101)
	w := postJSON(t, engine, http.MethodGet, "/api/v1/admin/materials/missing", "", csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("missing status = %d, body = %s", w.Code, w.Body)
	}
	data := decodeData(t, w.Body.Bytes())
	assertSnakeCaseKeys(t, "missing materials", data)
	if data["total"] != float64(1) {
		t.Fatalf("missing total = %v, want 1", data["total"])
	}
	items, _ := data["list"].([]any)
	item, _ := items[0].(map[string]any)
	assertSnakeCaseKeys(t, "missing material item", item)
	assertObjectKeys(t, "missing material item", item,
		"registration_id", "order_no", "event_id", "event_name", "group_id", "level",
		"dance_type", "player_no", "player_name", "contact_phone", "completed", "total",
		"missing_items", "material_deadline", "material_updated_at", "contacted", "contacted_at")
	if item["completed"] != float64(0) || item["total"] != float64(8) || item["contacted"] != false {
		t.Fatalf("item = %v", item)
	}
	missing, _ := item["missing_items"].([]any)
	if len(missing) != 8 {
		t.Fatalf("missing_items = %v", missing)
	}
	registrationID := int64(item["registration_id"].(float64))

	// 缺失项筛选命中；非法筛选值 → 40001。
	w = postJSON(t, engine, http.MethodGet,
		"/api/v1/admin/materials/missing?missing_item=audio", "", adminHeaders)
	if total := decodeData(t, w.Body.Bytes())["total"]; total != float64(1) {
		t.Fatalf("missing audio total = %v, want 1", total)
	}
	assertCode(t, postJSON(t, engine, http.MethodGet,
		"/api/v1/admin/materials/missing?deadline_state=locked", "", adminHeaders), 40001)

	// 标记已联系：cs 可操作；响应字段收敛。
	contactPath := fmt.Sprintf("/api/v1/admin/materials/missing/%d/contacted", registrationID)
	assertCode(t, postJSON(t, engine, http.MethodPost, contactPath, `{"note":"已电话提醒"}`, nil), 40101)
	assertCode(t, postJSON(t, engine, http.MethodPost, contactPath,
		fmt.Sprintf(`{"note":%q}`, strings.Repeat("催", 201)), adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodPost,
		"/api/v1/admin/materials/missing/999999/contacted", `{"note":"x"}`, adminHeaders), 40401)

	w = postJSON(t, engine, http.MethodPost, contactPath, `{"note":"已电话提醒"}`, csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("contacted status = %d, body = %s", w.Code, w.Body)
	}
	contacted := decodeData(t, w.Body.Bytes())
	assertObjectKeys(t, "contacted", contacted, "registration_id", "contacted")
	if contacted["contacted"] != true || contacted["registration_id"] != float64(registrationID) {
		t.Fatalf("contacted = %v", contacted)
	}

	// 名单回显已联系。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/materials/missing", "", adminHeaders)
	items, _ = decodeData(t, w.Body.Bytes())["list"].([]any)
	item, _ = items[0].(map[string]any)
	if item["contacted"] != true || item["contacted_at"] == nil {
		t.Fatalf("item after contacted = %v", item)
	}

	// 资料截止后锁定 → 41008。
	if _, err := db.SQL().ExecContext(context.Background(), `UPDATE events SET
		reg_start_at = '2018-12-01T00:00:00+08:00',
		reg_end_at = '2018-12-15T00:00:00+08:00',
		lottery_at = '2019-01-01T00:00:00+08:00',
		material_check_at = '2019-02-01T00:00:00+08:00',
		start_at = '2019-03-01T00:00:00+08:00' WHERE id = ?`, eventID); err != nil {
		t.Fatalf("move timeline: %v", err)
	}
	assertCode(t, postJSON(t, engine, http.MethodPost, contactPath, `{"note":"x"}`, adminHeaders), 41008)
}
