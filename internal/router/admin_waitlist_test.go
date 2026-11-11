package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/store"
)

// csAdminToken 按 cs 角色签发后台 token（只读权限）。
func csAdminToken(t *testing.T, signer *token.Signer) string {
	t.Helper()
	raw, err := signer.IssueAdmin(1, model.RoleCS)
	if err != nil {
		t.Fatalf("issue cs token: %v", err)
	}
	return raw
}

// mustRouterPaidOrder 构造一条 active 报名 + paid 订单（后台转报名关联用）。
func mustRouterPaidOrder(t *testing.T, db *store.DB, eventID, groupID, userID int64, playerNo, orderNo string) string {
	t.Helper()
	ctx := context.Background()
	res, err := db.SQL().ExecContext(ctx,
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, '关联选手', ?)`, playerNo, "hash-"+playerNo)
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
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO orders (order_no, registration_id, user_id, amount, status, paid_at)
		 VALUES (?, ?, ?, 98000, 'paid', '2027-01-01T10:00:00+08:00')`, orderNo, regID, userID); err != nil {
		t.Fatalf("insert order: %v", err)
	}
	return orderNo
}

// TestAdminWaitlistListHTTP 覆盖后台列表鉴权、角色、筛选与字段收敛。
func TestAdminWaitlistListHTTP(t *testing.T) {
	engine, signer, db := newWaitlistTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, auth, seasonID, "后台候补接口", 1)
	userID := mustInsertRouterUser(t, db, "o-admin-http-wait")
	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
		waitlistBody(groupID, "张小雨", routerWaitlistIDCard, "13812345678"),
		map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)})
	if w.Code != http.StatusOK {
		t.Fatalf("submit waitlist status = %d, body = %s", w.Code, w.Body)
	}

	// 未登录 → 40101。
	assertCode(t, getJSON(t, engine, "/api/v1/admin/waitlists", nil), 40101)

	// cs 只读可访问。
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	w = getJSON(t, engine, fmt.Sprintf("/api/v1/admin/waitlists?event_id=%d&status=waiting", eventID), csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Data struct {
			List          []map[string]any `json:"list"`
			Total         int              `json:"total"`
			ReleasedQuota int              `json:"released_quota"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode admin list: %v", err)
	}
	if raw.Data.Total != 1 || len(raw.Data.List) != 1 {
		t.Fatalf("admin list = %+v", raw.Data)
	}
	item := raw.Data.List[0]
	assertSnakeCaseKeys(t, "admin waitlist", item)
	assertObjectKeys(t, "admin waitlist", item,
		"waitlist_id", "position_no", "name", "id_card_masked", "institution", "phone",
		"status", "created_at", "notified_at", "handled_at")
	if item["phone"] != "13812345678" || item["id_card_masked"] == nil || item["status"] != "waiting" {
		t.Fatalf("admin item = %v", item)
	}
	for _, k := range []string{"id_card_no", "id_card_hash", "user_id"} {
		if _, ok := item[k]; ok {
			t.Fatalf("sensitive key %q exposed: %v", k, item)
		}
	}
	if raw.Data.ReleasedQuota != 0 {
		t.Fatalf("released_quota = %d, want 0 (full)", raw.Data.ReleasedQuota)
	}

	// 非法筛选 → 40001。
	assertCode(t, getJSON(t, engine, "/api/v1/admin/waitlists?status=bogus", csHeaders), 40001)
	assertCode(t, getJSON(t, engine, "/api/v1/admin/waitlists?event_id=abc", csHeaders), 40001)
}

// TestAdminWaitlistActionsHTTP 覆盖通知/移出/转报名（cs 无写权限）。
func TestAdminWaitlistActionsHTTP(t *testing.T) {
	engine, signer, db := newWaitlistTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "后台操作接口", 1)

	userA := mustInsertRouterUser(t, db, "o-admin-act-a")
	userB := mustInsertRouterUser(t, db, "o-admin-act-b")
	submit := func(user int64, idCard string) int64 {
		w := postJSON(t, engine, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
			waitlistBody(groupID, "张小雨", idCard, "13812345678"),
			map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, user)})
		if w.Code != http.StatusOK {
			t.Fatalf("submit waitlist status = %d, body = %s", w.Code, w.Body)
		}
		var resp struct {
			Data struct {
				WaitlistID int64 `json:"waitlist_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode submit: %v", err)
		}
		return resp.Data.WaitlistID
	}
	waitlistA := submit(userA, routerWaitlistIDCard)
	waitlistB := submit(userB, "110105201706150013")

	// cs 写操作 → 40301。
	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/waitlists/%d/notify", waitlistA),
		`{"note":"confirmed_intent"}`, csHeaders)
	assertCode(t, w, 40301)

	// 非法 note → 40001。
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/waitlists/%d/notify", waitlistA), `{"note":"maybe"}`, adminHeaders)
	assertCode(t, w, 40001)

	// 标记已通知 → 200，data.status = notified。
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/waitlists/%d/notify", waitlistA),
		`{"note":"confirmed_intent"}`, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("notify status = %d, body = %s", w.Code, w.Body)
	}
	var notified struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &notified); err != nil {
		t.Fatalf("decode notify: %v", err)
	}
	assertSnakeCaseKeys(t, "notify", notified.Data)
	if notified.Data["status"] != "notified" || notified.Data["waitlist_id"] != float64(waitlistA) {
		t.Fatalf("notify data = %v", notified.Data)
	}

	// 转报名：已支付订单 → converted。
	orderNo := mustRouterPaidOrder(t, db, eventID, groupID, userB, "PAYW0001", "ORDER-WAIT-OK-1")
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/waitlists/%d/convert", waitlistB),
		fmt.Sprintf(`{"order_no":%q}`, orderNo), adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("convert status = %d, body = %s", w.Code, w.Body)
	}
	var converted struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &converted); err != nil {
		t.Fatalf("decode convert: %v", err)
	}
	if converted.Data["status"] != "converted" {
		t.Fatalf("convert data = %v", converted.Data)
	}

	// 移出队列 → removed；重复移出 → 41013。
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/waitlists/%d/remove", waitlistA),
		`{"reason":"多次联系不上"}`, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("remove status = %d, body = %s", w.Code, w.Body)
	}
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/waitlists/%d/remove", waitlistA),
		`{"reason":"再次移出"}`, adminHeaders)
	assertCode(t, w, 41013)

	// 不存在 → 40401；非法 ID → 40001。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/waitlists/9999/notify",
		`{"note":"confirmed_intent"}`, adminHeaders)
	assertCode(t, w, 40401)
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/waitlists/abc/notify",
		`{"note":"confirmed_intent"}`, adminHeaders)
	assertCode(t, w, 40001)
}
