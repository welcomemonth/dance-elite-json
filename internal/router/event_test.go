package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newPublicEventTestEngine 装配后台管理 + 小程序赛事查询路由的完整测试引擎。
func newPublicEventTestEngine(t *testing.T) (*gin.Engine, *token.Signer) {
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
	return New(Deps{
		Stores:       stores,
		AdminAuth:    service.NewAdminAuthService(stores, signer),
		Seasons:      service.NewSeasonService(stores),
		Events:       service.NewEventService(stores),
		PublicEvents: service.NewPublicEventService(stores),
		Tokens:       signer,
	}), signer
}

// getJSON 发起 GET 请求并返回响应记录器。
func getJSON(t *testing.T, engine *gin.Engine, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// offsetTime 返回 now 偏移 hours 小时后的 ISO 8601（+08:00）字符串。
func offsetTime(hours time.Duration) string {
	return time.Now().Add(hours * time.Hour).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02T15:04:05-07:00")
}

// publicEventBody 构造发布赛事的请求体，报名窗口为 [now-1h, now+24h]（展示为报名中）。
func publicEventBody(seasonID int64, name string) string {
	return fmt.Sprintf(`{
		"name": %q, "season_id": %d, "level": "grade_a",
		"city": "上海", "venue": "上海国际舞蹈中心", "event_date": "2027-03-15",
		"fee": 98000, "publish": true,
		"timeline": {
			"reg_start_at": %q,
			"reg_end_at": %q,
			"lottery_at": %q,
			"material_check_at": %q,
			"start_at": %q
		},
		"groups": [
			{"level": "U11", "dance_type": "classical", "quota": 40},
			{"level": "U11", "dance_type": "folk", "quota": 40}
		]
	}`, name, seasonID,
		offsetTime(-1), offsetTime(24), offsetTime(48), offsetTime(72), offsetTime(96))
}

// mustCreatePublishedEvent 通过后台接口创建并发布赛事，返回赛事 ID。
func mustCreatePublishedEvent(t *testing.T, engine *gin.Engine, auth map[string]string, seasonID int64, name string) int64 {
	t.Helper()
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/events",
		publicEventBody(seasonID, name), auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create event %s status = %d, body = %s", name, w.Code, w.Body)
	}
	var created struct {
		Data struct {
			EventID int64 `json:"event_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return created.Data.EventID
}

// rawPublicList 是赛事列表响应的原始 JSON 结构（键断言用，防假绿）。
type rawPublicList struct {
	Code    int `json:"code"`
	Message string
	Data    struct {
		List []map[string]any `json:"list"`
	} `json:"data"`
}

func TestPublicEventList(t *testing.T) {
	engine, _ := newPublicEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)

	mustCreatePublishedEvent(t, engine, auth, seasonID, "报名中赛事")
	mustCreatePublishedEvent(t, engine, auth, seasonID, "另一个赛事")

	// draft 不可见。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/events",
		fmt.Sprintf(`{"name":"草稿赛事","season_id":%d,"level":"grade_a","venue":"场馆","event_date":"2027-03-15","fee":98000,
			"timeline":{"reg_start_at":%q,"reg_end_at":%q,"lottery_at":%q,"material_check_at":%q,"start_at":%q},
			"groups":[{"level":"U11","dance_type":"classical","quota":40}]}`,
			seasonID, offsetTime(-1), offsetTime(24), offsetTime(48), offsetTime(72), offsetTime(96)), auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create draft status = %d, body = %s", w.Code, w.Body)
	}

	w = getJSON(t, engine, "/api/v1/events", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body)
	}
	var raw rawPublicList
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if raw.Code != 0 {
		t.Fatalf("list code = %d, body = %s", raw.Code, w.Body)
	}
	if len(raw.Data.List) != 2 {
		t.Fatalf("list len = %d, want 2 (draft excluded), body = %s", len(raw.Data.List), w.Body)
	}

	// 列表项：snake_case 键、无内部字段、list 非 null。
	internalKeys := []string{"score_rules", "prize_rules", "audio_review_token", "season_id", "exclude_from_prize", "Status", "Name"}
	for _, item := range raw.Data.List {
		assertSnakeCaseKeys(t, "public event", item)
		assertObjectKeys(t, "public event", item,
			"id", "name", "level", "city", "venue", "event_date", "cover_url",
			"fee", "status", "reg_end_at", "group_summary", "registered_by_me")
		for _, k := range internalKeys {
			if _, ok := item[k]; ok {
				t.Fatalf("internal key %q exposed in list item: %v", k, item)
			}
		}
		if item["status"] != "registering" {
			t.Fatalf("status = %v, want registering", item["status"])
		}
		summary, ok := item["group_summary"].([]any)
		if !ok || len(summary) != 2 {
			t.Fatalf("group_summary = %v", item["group_summary"])
		}
		assertSnakeCaseKeys(t, "group summary", summary[0].(map[string]any))
		if item["registered_by_me"] != false {
			t.Fatalf("anonymous registered_by_me = %v", item["registered_by_me"])
		}
	}

	// 状态筛选：upcoming（无）→ 空列表而非 null。
	w = getJSON(t, engine, "/api/v1/events?status=upcoming", nil)
	var filtered rawPublicList
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if filtered.Code != 0 || filtered.Data.List == nil {
		t.Fatalf("filtered list = %+v, want code 0 with non-nil empty list", filtered)
	}

	// 无效 id 详情 → 40001。
	w = getJSON(t, engine, "/api/v1/events/abc", nil)
	assertCode(t, w, 40001)
}

func TestPublicEventDetail(t *testing.T) {
	engine, _ := newPublicEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "报名中赛事")

	w := getJSON(t, engine, "/api/v1/events/"+itoa(eventID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Code int `json:"code"`
		Data map[string]any
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if raw.Code != 0 {
		t.Fatalf("detail code = %d", raw.Code)
	}
	item := raw.Data
	assertSnakeCaseKeys(t, "public event detail", item)
	assertObjectKeys(t, "public event detail", item,
		"id", "name", "level", "city", "venue", "event_date", "cover_url",
		"fee", "status", "timeline", "groups", "refund_policy",
		"work_duration_limit", "process_desc", "honor_desc")
	for _, k := range []string{"score_rules", "prize_rules", "audio_review_token", "season_id"} {
		if _, ok := item[k]; ok {
			t.Fatalf("internal key %q exposed in detail: %v", k, item)
		}
	}

	timeline, ok := item["timeline"].(map[string]any)
	if !ok {
		t.Fatalf("timeline = %v", item["timeline"])
	}
	assertObjectKeys(t, "timeline", timeline,
		"reg_start_at", "reg_end_at", "lottery_at", "material_check_at", "start_at")

	groups, ok := item["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("groups = %v", item["groups"])
	}
	group0 := groups[0].(map[string]any)
	assertSnakeCaseKeys(t, "public event group", group0)
	assertObjectKeys(t, "public event group", group0,
		"group_id", "level", "dance_type", "birth_year_from", "birth_year_to",
		"quota", "registered", "remaining", "enabled")
	if group0["remaining"] != group0["quota"].(float64)-group0["registered"].(float64) {
		t.Fatalf("remaining inconsistent: %v", group0)
	}

	// 已下线赛事详情 → 40401。
	w = postJSON(t, engine, http.MethodPost,
		"/api/v1/admin/events/"+itoa(eventID)+"/offline", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("offline status = %d, body = %s", w.Code, w.Body)
	}
	w = getJSON(t, engine, "/api/v1/events/"+itoa(eventID), nil)
	assertCode(t, w, 40401)

	// 不存在 → 40401。
	w = getJSON(t, engine, "/api/v1/events/9999", nil)
	assertCode(t, w, 40401)
}

// TestPublicEventListOptionalAuth 验证可选登录中间件：非法 token 不拒绝（按匿名处理），
// 合法 token 解析出 user_id（registered_by_me 为 false，因该用户无报名）。
func TestPublicEventListOptionalAuth(t *testing.T) {
	engine, signer := newPublicEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	mustCreatePublishedEvent(t, engine, auth, seasonID, "报名中赛事")

	// 非法 token：按匿名放行，不 401。
	w := getJSON(t, engine, "/api/v1/events", map[string]string{
		"Authorization": "Bearer garbage",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("garbage token status = %d, body = %s", w.Code, w.Body)
	}

	// 合法用户 token：放行且能走完 registered_by_me 查询（无报名 → false）。
	userToken, err := signer.IssueUser(123, "o-test")
	if err != nil {
		t.Fatalf("issue user token: %v", err)
	}
	w = getJSON(t, engine, "/api/v1/events", map[string]string{
		"Authorization": "Bearer " + userToken,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("user token status = %d, body = %s", w.Code, w.Body)
	}
	var raw rawPublicList
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.Data.List) != 1 || raw.Data.List[0]["registered_by_me"] != false {
		t.Fatalf("user token list = %+v", raw.Data.List)
	}
}

// assertCode 断言响应包装中的业务错误码。
func assertCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v, body = %s", err, w.Body)
	}
	if resp.Code != want {
		t.Fatalf("code = %d, want %d, body = %s", resp.Code, want, w.Body)
	}
}
