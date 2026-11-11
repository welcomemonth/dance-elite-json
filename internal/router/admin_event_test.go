package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newAdminEventTestEngine 装配后台认证 + 赛季/赛事管理路由的完整测试引擎。
func newAdminEventTestEngine(t *testing.T) (*gin.Engine, *token.Signer) {
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
		Stores:    stores,
		AdminAuth: service.NewAdminAuthService(stores, signer),
		Seasons:   service.NewSeasonService(stores),
		Events:    service.NewEventService(stores),
		Tokens:    signer,
	}), signer
}

// adminToken 通过登录接口换取超管 token。
func adminToken(t *testing.T, engine *gin.Engine) string {
	t.Helper()
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"admin","password":"Init@12345"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	return resp.Data.Token
}

const adminEventBody = `{
	"name": "2027 上海甲级赛", "season_id": %d, "level": "grade_a",
	"city": "上海", "venue": "上海国际舞蹈中心", "event_date": "2027-03-15",
	"fee": 98000,
	"timeline": {
		"reg_start_at": "2027-01-01T09:00:00+08:00",
		"reg_end_at": "2027-02-01T18:00:00+08:00",
		"lottery_at": "2027-02-05T10:00:00+08:00",
		"material_check_at": "2027-03-01T18:00:00+08:00",
		"start_at": "2027-03-15T09:00:00+08:00"
	},
	"groups": [
		{"level": "U11", "dance_type": "classical", "quota": 40},
		{"level": "U11", "dance_type": "folk", "quota": 40}
	]
}`

const adminEventUpdateBody = `{
	"name": "2027 上海甲级赛（春）", "level": "grade_a",
	"city": "上海", "venue": "上海国际舞蹈中心", "event_date": "2027-03-15",
	"fee": 98000,
	"timeline": {
		"reg_start_at": "2027-01-01T09:00:00+08:00",
		"reg_end_at": "2027-02-01T18:00:00+08:00",
		"lottery_at": "2027-02-05T10:00:00+08:00",
		"material_check_at": "2027-03-01T18:00:00+08:00",
		"start_at": "2027-03-15T09:00:00+08:00"
	},
	"groups": [
		{"level": "U11", "dance_type": "classical", "quota": 50}
	]
}`

// setupSeason 创建并激活赛季，返回赛季 ID。
func setupSeason(t *testing.T, engine *gin.Engine, auth map[string]string) int64 {
	t.Helper()
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/seasons",
		`{"name":"2027 赛季","start_date":"2027-01-01","end_date":"2027-12-31"}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create season status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode season: %v", err)
	}
	w = postJSON(t, engine, http.MethodPut, "/api/v1/admin/seasons/"+itoa(created.Data.ID),
		`{"name":"2027 赛季","status":"active"}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("activate season status = %d, body = %s", w.Code, w.Body)
	}
	return created.Data.ID
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// assertSnakeCaseKeys 断言 JSON 对象的所有键均为 snake_case（全小写）。
// 回归保护：领域模型（如 model.Season）若被直接序列化会输出 PascalCase 键，
// encoding/json 解码大小写不敏感，普通结构体断言会假绿。
func assertSnakeCaseKeys(t *testing.T, label string, obj map[string]any) {
	t.Helper()
	for key := range obj {
		if key != strings.ToLower(key) {
			t.Fatalf("%s 存在非 snake_case 键 %q（完整对象：%v）", label, key, obj)
		}
	}
}

// assertObjectKeys 断言对象包含指定键。
func assertObjectKeys(t *testing.T, label string, obj map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := obj[key]; !ok {
			t.Fatalf("%s 缺少键 %q（完整对象：%v）", label, key, obj)
		}
	}
}

func TestAdminSeasonRoutes(t *testing.T) {
	engine, _ := newAdminEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}

	// 未登录 → 40101。
	w := postJSON(t, engine, http.MethodGet, "/api/v1/admin/seasons", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", w.Code)
	}

	// cs 角色写操作 → 40301（token 直接按角色签发，RequireRole 只校验 role claim）。
	csRaw, err := token.NewSigner("test-secret", time.Hour).IssueAdmin(2, "cs")
	if err != nil {
		t.Fatalf("IssueAdmin: %v", err)
	}
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/seasons",
		`{"name":"x"}`, map[string]string{"Authorization": "Bearer " + csRaw})
	if w.Code != http.StatusForbidden {
		t.Fatalf("cs write status = %d, body = %s", w.Code, w.Body)
	}
	var resp httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Code != httpx.CodeForbidden {
		t.Fatalf("cs write code = %+v/%v, want 40301", resp, err)
	}

	// 完整流程：创建 → 激活 → 列表。
	seasonID := setupSeason(t, engine, auth)

	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/seasons", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("list seasons status = %d", w.Code)
	}
	var list struct {
		Data struct {
			List []struct {
				ID     int64  `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Data.List) != 1 || list.Data.List[0].ID != seasonID || list.Data.List[0].Status != "active" {
		t.Fatalf("seasons = %+v", list.Data.List)
	}

	// 原始 JSON 键必须是 snake_case（防止 DTO 缺失 json tag 的假绿）。
	var rawSeasons struct {
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rawSeasons); err != nil {
		t.Fatalf("decode raw seasons: %v", err)
	}
	if len(rawSeasons.Data.List) != 1 {
		t.Fatalf("raw seasons = %+v", rawSeasons.Data.List)
	}
	assertSnakeCaseKeys(t, "season", rawSeasons.Data.List[0])
	assertObjectKeys(t, "season", rawSeasons.Data.List[0],
		"id", "name", "start_date", "end_date", "status", "group_rules", "created_at")

	// 非法日期 → 40001。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/seasons",
		`{"name":"bad","start_date":"2027-13-01"}`, auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad date status = %d, want 400", w.Code)
	}
}

func TestAdminEventLifecycleRoutes(t *testing.T) {
	engine, _ := newAdminEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)

	// 创建（草稿）。
	body := fmt.Sprintf(adminEventBody, seasonID)
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/events", body, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create event status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			EventID int64  `json:"event_id"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Data.Status != "draft" {
		t.Fatalf("status = %q, want draft", created.Data.Status)
	}
	eventID := created.Data.EventID

	// 时间线乱序 → 40001。
	badTimeline := `{"name":"x","season_id":%d,"level":"grade_a","venue":"v","event_date":"2027-03-15","fee":0,` +
		`"timeline":{"reg_start_at":"2027-02-01T09:00:00+08:00","reg_end_at":"2027-01-01T09:00:00+08:00",` +
		`"lottery_at":"2027-02-05T10:00:00+08:00","material_check_at":"2027-03-01T18:00:00+08:00",` +
		`"start_at":"2027-03-15T09:00:00+08:00"},"groups":[{"level":"U11","dance_type":"folk","quota":10}]}`
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events", fmt.Sprintf(badTimeline, seasonID), auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad timeline status = %d, body = %s", w.Code, w.Body)
	}

	// 发布 → registering。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events/"+itoa(eventID)+"/publish", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body = %s", w.Code, w.Body)
	}
	// 重复发布 → 41012。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events/"+itoa(eventID)+"/publish", "", auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("republish status = %d, want 400 (41012)", w.Code)
	}
	var errResp httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil || errResp.Code != httpx.CodeEventStateNotAllowed {
		t.Fatalf("republish code = %+v/%v, want 41012", errResp, err)
	}

	// 列表：进度含两个组别，出生年份从赛季规则复制（U11=2016-2018）。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/events?status=registering", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("list events status = %d", w.Code)
	}
	var list struct {
		Data struct {
			List []struct {
				EventID int64  `json:"id"`
				Status  string `json:"status"`
				Groups  []struct {
					GroupID       int64 `json:"group_id"`
					BirthYearFrom int   `json:"birth_year_from"`
					BirthYearTo   int   `json:"birth_year_to"`
					Quota         int   `json:"quota"`
					Registered    int   `json:"registered"`
				} `json:"groups"`
			} `json:"list"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Data.Total != 1 || len(list.Data.List[0].Groups) != 2 {
		t.Fatalf("list = %+v", list.Data)
	}
	g0 := list.Data.List[0].Groups[0]
	if g0.BirthYearFrom != 2016 || g0.BirthYearTo != 2018 {
		t.Fatalf("birth years = %d-%d, want 2016-2018", g0.BirthYearFrom, g0.BirthYearTo)
	}
	groupID := g0.GroupID

	// 原始 JSON 键必须是 snake_case（防止嵌入式领域模型输出 PascalCase 的假绿）。
	var rawEvents struct {
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rawEvents); err != nil {
		t.Fatalf("decode raw events: %v", err)
	}
	if len(rawEvents.Data.List) != 1 {
		t.Fatalf("raw events = %+v", rawEvents.Data.List)
	}
	item := rawEvents.Data.List[0]
	assertSnakeCaseKeys(t, "event", item)
	assertObjectKeys(t, "event", item,
		"id", "season_id", "name", "level", "event_date", "fee", "status",
		"reg_start_at", "start_at", "score_rules", "prize_rules",
		"exclude_from_prize", "audio_review_token", "groups")
	groupsRaw, ok := item["groups"].([]any)
	if !ok || len(groupsRaw) != 2 {
		t.Fatalf("event.groups = %v", item["groups"])
	}
	groupObj, ok := groupsRaw[0].(map[string]any)
	if !ok {
		t.Fatalf("event.groups[0] = %v", groupsRaw[0])
	}
	assertSnakeCaseKeys(t, "event group", groupObj)
	assertObjectKeys(t, "event group", groupObj,
		"group_id", "level", "dance_type", "birth_year_from", "birth_year_to",
		"quota", "registered", "enabled")

	// 已发布：修改时间线 → 40001。
	badUpdate := `{"name":"x","level":"grade_a","venue":"v","event_date":"2027-03-16","fee":98000,` +
		`"timeline":{"reg_start_at":"2027-01-01T09:00:00+08:00","reg_end_at":"2027-02-01T18:00:00+08:00",` +
		`"lottery_at":"2027-02-05T10:00:00+08:00","material_check_at":"2027-03-01T18:00:00+08:00",` +
		`"start_at":"2027-03-20T09:00:00+08:00"},"groups":[{"level":"U11","dance_type":"classical","quota":40}]}`
	w = postJSON(t, engine, http.MethodPut, "/api/v1/admin/events/"+itoa(eventID), badUpdate, auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("timeline change status = %d, body = %s", w.Code, w.Body)
	}

	// 已发布：编辑调大名额（50）成功。
	w = postJSON(t, engine, http.MethodPut, "/api/v1/admin/events/"+itoa(eventID), adminEventUpdateBody, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("update event status = %d, body = %s", w.Code, w.Body)
	}

	// 调名额接口：调小 → 40001；调大 → 成功且 remaining 正确。
	w = postJSON(t, engine, http.MethodPut,
		"/api/v1/admin/events/"+itoa(eventID)+"/groups/"+itoa(groupID)+"/quota",
		`{"quota":30}`, auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("decrease quota status = %d, body = %s", w.Code, w.Body)
	}
	w = postJSON(t, engine, http.MethodPut,
		"/api/v1/admin/events/"+itoa(eventID)+"/groups/"+itoa(groupID)+"/quota",
		`{"quota":60}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("increase quota status = %d, body = %s", w.Code, w.Body)
	}
	var quota struct {
		Data struct {
			Quota     int `json:"quota"`
			Remaining int `json:"remaining"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &quota); err != nil {
		t.Fatalf("decode quota: %v", err)
	}
	if quota.Data.Quota != 60 || quota.Data.Remaining != 60 {
		t.Fatalf("quota = %+v", quota.Data)
	}

	// 关闭/重开组别。
	w = postJSON(t, engine, http.MethodPost,
		"/api/v1/admin/events/"+itoa(eventID)+"/groups/"+itoa(groupID)+"/close", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("close group status = %d, body = %s", w.Code, w.Body)
	}
	w = postJSON(t, engine, http.MethodPost,
		"/api/v1/admin/events/"+itoa(eventID)+"/groups/"+itoa(groupID)+"/reopen", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("reopen group status = %d, body = %s", w.Code, w.Body)
	}

	// 关闭报名 → reg_closed；再关闭 → 41012。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events/"+itoa(eventID)+"/close-registration", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("close-registration status = %d, body = %s", w.Code, w.Body)
	}
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events/"+itoa(eventID)+"/close-registration", "", auth)
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil || errResp.Code != httpx.CodeEventStateNotAllowed {
		t.Fatalf("re-close code = %+v/%v, want 41012", errResp, err)
	}

	// 下架 → offline。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events/"+itoa(eventID)+"/offline", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("offline status = %d, body = %s", w.Code, w.Body)
	}
	// 下架后再调名额 → 41012。
	w = postJSON(t, engine, http.MethodPut,
		"/api/v1/admin/events/"+itoa(eventID)+"/groups/"+itoa(groupID)+"/quota",
		`{"quota":70}`, auth)
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil || errResp.Code != httpx.CodeEventStateNotAllowed {
		t.Fatalf("offline adjust code = %+v/%v, want 41012", errResp, err)
	}
}

func TestAdminEventCreateRequiresActiveSeasonRoute(t *testing.T) {
	engine, _ := newAdminEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}

	// 未激活的赛季创建赛事 → 40901。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/seasons",
		`{"name":"2028 赛季"}`, auth)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events",
		fmt.Sprintf(adminEventBody, created.Data.ID), auth)
	if w.Code != http.StatusConflict {
		t.Fatalf("inactive season create status = %d, body = %s", w.Code, w.Body)
	}
	var resp httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Code != httpx.CodeConflict {
		t.Fatalf("code = %+v/%v, want 40901", resp, err)
	}

	// 不存在的赛事 → 40401。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/events/9999/publish", "", auth)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing event status = %d, want 404", w.Code)
	}
}

func TestAdminEventDetailRoute(t *testing.T) {
	engine, _ := newAdminEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)

	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/events",
		fmt.Sprintf(adminEventBody, seasonID), auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create event status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			EventID int64 `json:"event_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// 详情：结构与列表项一致，键为 snake_case。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/events/"+itoa(created.Data.EventID), "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("get detail status = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	assertSnakeCaseKeys(t, "event detail", raw.Data)
	assertObjectKeys(t, "event detail", raw.Data, "id", "season_id", "groups")
	if groups, ok := raw.Data["groups"].([]any); !ok || len(groups) != 2 {
		t.Fatalf("detail groups = %v", raw.Data["groups"])
	}

	// 不存在 → 40401。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/events/9999", "", auth)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing detail status = %d, want 404", w.Code)
	}
}

func TestAdminSeasonEmptyListReturnsArray(t *testing.T) {
	engine, _ := newAdminEventTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}

	// 空列表必须返回 [] 而非 null（前端按数组处理）。
	w := postJSON(t, engine, http.MethodGet, "/api/v1/admin/seasons", "", auth)
	if w.Code != http.StatusOK {
		t.Fatalf("list seasons status = %d", w.Code)
	}
	var raw struct {
		Data struct {
			List []any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if raw.Data.List == nil || len(raw.Data.List) != 0 {
		t.Fatalf("list = %v, want empty non-nil array", raw.Data.List)
	}
	if !strings.Contains(w.Body.String(), `"list":[]`) {
		t.Fatalf("raw body = %s, want \"list\":[]", w.Body)
	}
}
