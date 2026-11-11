package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newWaitlistTestEngine 装配含候补路由的测试引擎（用户端 + 后台）。
func newWaitlistTestEngine(t *testing.T) (*gin.Engine, *token.Signer, *store.DB) {
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
	return New(Deps{
		Stores:        stores,
		AdminAuth:     service.NewAdminAuthService(stores, signer),
		Seasons:       service.NewSeasonService(stores),
		Events:        service.NewEventService(stores),
		PublicEvents:  service.NewPublicEventService(stores),
		Registrations: service.NewRegistrationService(stores, players),
		Waitlists:     service.NewWaitlistService(stores, players),
		Tokens:        signer,
	}), signer, db
}

// mustCreateWaitlistEvent 创建报名窗口为 [now-1h, now+24h] 的发布赛事并占满名额，
// 返回赛事与组别 ID。
func mustCreateWaitlistEvent(t *testing.T, engine *gin.Engine, db *store.DB, auth map[string]string, seasonID int64, name string, quota int) (eventID, groupID int64) {
	t.Helper()
	body := fmt.Sprintf(`{
		"name": %q, "season_id": %d, "level": "grade_a",
		"city": "上海", "venue": "上海国际舞蹈中心", "event_date": "2027-03-15",
		"fee": 98000, "publish": true,
		"timeline": {
			"reg_start_at": %q, "reg_end_at": %q, "lottery_at": %q,
			"material_check_at": %q, "start_at": %q
		},
		"groups": [{"level": "U11", "dance_type": "classical", "quota": %d}]
	}`, name, seasonID, offsetTime(-1), offsetTime(24), offsetTime(48), offsetTime(72), offsetTime(96), quota)
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/events", body, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create waitlist event status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			EventID int64 `json:"event_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	eventID = created.Data.EventID
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM event_groups WHERE event_id = ? AND level = 'U11' AND dance_type = 'classical'`,
		eventID).Scan(&groupID); err != nil {
		t.Fatalf("query group: %v", err)
	}
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE event_groups SET taken = quota WHERE id = ?`, groupID); err != nil {
		t.Fatalf("fill group: %v", err)
	}
	return eventID, groupID
}

// waitlistBody 构造提交候补请求体。
func waitlistBody(groupID int64, name, idCard, phone string) string {
	return fmt.Sprintf(`{"group_id":%d,"name":%q,"id_card_no":%q,"phone":%q}`, groupID, name, idCard, phone)
}

// 合法身份证（2016-03-07 生，U11）。
const routerWaitlistIDCard = "110105201603070010"

// TestSubmitWaitlistHTTP 覆盖鉴权、成功响应字段与重复排队错误码。
func TestSubmitWaitlistHTTP(t *testing.T) {
	engine, signer, db := newWaitlistTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, auth, seasonID, "候补接口赛事", 1)
	userID := mustInsertRouterUser(t, db, "o-http-wait")

	// 未登录 → 40101。
	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
		waitlistBody(groupID, "张小雨", routerWaitlistIDCard, "13812345678"), nil)
	assertCode(t, w, 40101)

	// 成功：snake_case 键，不暴露证件号/哈希。
	userHeaders := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
		waitlistBody(groupID, "张小雨", routerWaitlistIDCard, "13812345678"), userHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("submit waitlist status = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if raw.Code != 0 {
		t.Fatalf("code = %d, body = %s", raw.Code, w.Body)
	}
	assertSnakeCaseKeys(t, "submit waitlist", raw.Data)
	assertObjectKeys(t, "submit waitlist", raw.Data, "waitlist_id", "position_no", "status")
	if raw.Data["position_no"] != float64(1) || raw.Data["status"] != "waiting" {
		t.Fatalf("submit waitlist data = %v", raw.Data)
	}
	for _, k := range []string{"id_card_no", "id_card_hash", "id_card_masked", "user_id"} {
		if _, ok := raw.Data[k]; ok {
			t.Fatalf("internal key %q exposed: %v", k, raw.Data)
		}
	}

	// 同证件号重复排队 → 41010。
	otherUser := mustInsertRouterUser(t, db, "o-http-wait-2")
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
		waitlistBody(groupID, "张小雨", routerWaitlistIDCard, "13912345678"),
		map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, otherUser)})
	assertCode(t, w, 41010)

	// 名额未满 → 40001。
	freeEventID, freeGroupID := mustCreateWaitlistEvent(t, engine, db, auth, seasonID, "未满赛事", 5)
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE event_groups SET taken = 0 WHERE id = ?`, freeGroupID); err != nil {
		t.Fatalf("reset taken: %v", err)
	}
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", freeEventID),
		waitlistBody(freeGroupID, "李小雨", "110105201706150013", "13812345678"), userHeaders)
	assertCode(t, w, 40001)

	// 非法参数 → 40001。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/events/abc/waitlists",
		waitlistBody(groupID, "张小雨", routerWaitlistIDCard, "13812345678"), userHeaders)
	assertCode(t, w, 40001)
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
		`{"name":"张小雨"}`, userHeaders)
	assertCode(t, w, 40001)
}

// TestMyWaitlistsHTTP 覆盖「我的候补」与自助取消接口。
func TestMyWaitlistsHTTP(t *testing.T) {
	engine, signer, db := newWaitlistTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, auth, seasonID, "我的候补接口", 1)
	userID := mustInsertRouterUser(t, db, "o-http-mine")
	userHeaders := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}

	// 未登录 → 40101。
	assertCode(t, getJSON(t, engine, "/api/v1/waitlists", nil), 40101)

	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/waitlists", eventID),
		waitlistBody(groupID, "张小雨", routerWaitlistIDCard, "13812345678"), userHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("submit status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			WaitlistID int64 `json:"waitlist_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode submit: %v", err)
	}

	w = getJSON(t, engine, "/api/v1/waitlists", userHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body)
	}
	var list struct {
		Data struct {
			List  []map[string]any `json:"list"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Data.Total != 1 || len(list.Data.List) != 1 {
		t.Fatalf("my waitlists = %+v", list.Data)
	}
	assertSnakeCaseKeys(t, "my waitlist", list.Data.List[0])
	assertObjectKeys(t, "my waitlist", list.Data.List[0],
		"waitlist_id", "event_id", "event_name", "level", "dance_type", "position_no", "status", "created_at")
	for _, k := range []string{"phone", "id_card_no", "id_card_masked"} {
		if _, ok := list.Data.List[0][k]; ok {
			t.Fatalf("sensitive key %q exposed in my waitlist", k)
		}
	}

	// 取消 → data null；重复取消幂等。
	for i := 0; i < 2; i++ {
		w = postJSON(t, engine, http.MethodDelete,
			fmt.Sprintf("/api/v1/waitlists/%d", created.Data.WaitlistID), "", userHeaders)
		if w.Code != http.StatusOK {
			t.Fatalf("cancel #%d status = %d, body = %s", i, w.Code, w.Body)
		}
		var resp struct {
			Code int `json:"code"`
			Data any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode cancel: %v", err)
		}
		if resp.Code != 0 || resp.Data != nil {
			t.Fatalf("cancel resp = %+v", resp)
		}
	}

	// 非本人取消 → 40401。
	other := mustInsertRouterUser(t, db, "o-http-mine-other")
	w = postJSON(t, engine, http.MethodDelete,
		fmt.Sprintf("/api/v1/waitlists/%d", created.Data.WaitlistID), "",
		map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, other)})
	assertCode(t, w, 40401)

	// 非法 ID → 40001。
	w = postJSON(t, engine, http.MethodDelete, "/api/v1/waitlists/abc", "", userHeaders)
	assertCode(t, w, 40001)
}
