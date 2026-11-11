package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// manualRegBody 构造人工报名请求体（护照，出生年份落在 U11）。
func manualRegBody(eventID, groupID int64, passport string) string {
	return fmt.Sprintf(`{
		"event_id": %d, "group_id": %d, "player_name": "Emma Wilson",
		"id_type": "passport", "id_card_no": %q, "birth_date": "2017-05-01",
		"nationality": "美国", "institution": "国际舞蹈学校",
		"contact_phone": "13800000001", "charge_type": "offline_paid",
		"amount": 98000, "remark": "特殊证件登记"
	}`, eventID, groupID, passport)
}

// TestManualRegistrationHTTP 覆盖人工报名接口的角色限制、响应字段与错误码。
func TestManualRegistrationHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	_, csHeaders := seedCSAdmin(t, db, signer)
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "人工报名接口赛事", 5)
	// mustCreateWaitlistEvent 会把名额填满（候补前置），人工报名需要剩余名额。
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE event_groups SET taken = 0 WHERE id = ?`, groupID); err != nil {
		t.Fatalf("reset taken: %v", err)
	}
	path := "/api/v1/admin/manual-registrations"

	// 未登录 → 40101；cs → 40301。
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		manualRegBody(eventID, groupID, "P-HTTP1"), nil), 40101)
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		manualRegBody(eventID, groupID, "P-HTTP1"), csHeaders), 40301)

	// 成功：响应字段收敛。
	w := postJSON(t, engine, http.MethodPost, path, manualRegBody(eventID, groupID, "P-HTTP1"), adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("create manual reg status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode manual reg: %v", err)
	}
	assertSnakeCaseKeys(t, "manual registration", created.Data)
	assertObjectKeys(t, "manual registration", created.Data, "registration_id", "order_no", "player_no")
	if created.Data["order_no"] == "" {
		t.Fatalf("manual reg data = %v", created.Data)
	}

	// 重复报名 → 41004；超龄 → 41002。
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		manualRegBody(eventID, groupID, "P-HTTP1"), adminHeaders), 41004)
	overage := fmt.Sprintf(`{
		"event_id": %d, "group_id": %d, "player_name": "Older",
		"id_type": "passport", "id_card_no": "P-OLD", "birth_date": "2000-01-01",
		"contact_phone": "13800000002", "charge_type": "free"
	}`, eventID, groupID)
	assertCode(t, postJSON(t, engine, http.MethodPost, path, overage, adminHeaders), 41002)

	// 参数缺失/收费方式非法 → 40001；赛事不存在 → 40401。
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		`{"event_id": 1}`, adminHeaders), 40001)
	badCharge := fmt.Sprintf(`{
		"event_id": %d, "group_id": %d, "player_name": "Emma",
		"id_type": "passport", "id_card_no": "P-HTTP2", "birth_date": "2017-05-01",
		"contact_phone": "13800000003", "charge_type": "bogus"
	}`, eventID, groupID)
	assertCode(t, postJSON(t, engine, http.MethodPost, path, badCharge, adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		manualRegBody(999999, groupID, "P-HTTP3"), adminHeaders), 40401)
}
