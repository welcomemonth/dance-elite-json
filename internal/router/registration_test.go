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

// newRegistrationTestEngine 装配含报名下单路由的测试引擎，并返回签名器与底层 DB。
func newRegistrationTestEngine(t *testing.T) (*gin.Engine, *token.Signer, *store.DB) {
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
		Tokens:        signer,
	}), signer, db
}

// userTokenFor 为已插入的用户签发小程序 token（user_id 写入 JWT）。
func userTokenFor(t *testing.T, signer *token.Signer, userID int64) string {
	t.Helper()
	raw, err := signer.IssueUser(userID, fmt.Sprintf("o-test-%d", userID))
	if err != nil {
		t.Fatalf("issue user token: %v", err)
	}
	return raw
}

func mustInsertRouterUser(t *testing.T, db *store.DB, openid string) int64 {
	t.Helper()
	res, err := db.SQL().ExecContext(context.Background(), `INSERT INTO users (openid) VALUES (?)`, openid)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("user id: %v", err)
	}
	return id
}

// 与 service 测试同一手工核算合法身份证（2016-03-07 男，U11）。
const routerValidIDCard = "110105201603070010"

func submitBody(idCard, danceType, key string) string {
	return fmt.Sprintf(`{
		"player_name": "王小舞",
		"id_card_no": %q,
		"institution": "小天鹅舞蹈工作室",
		"dance_type": %q,
		"idempotency_key": %q
	}`, idCard, danceType, key)
}

func TestSubmitRegistrationAuthRequired(t *testing.T) {
	engine, _, _ := newRegistrationTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "鉴权赛事")

	// 无 token → 40101。
	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID), submitBody(routerValidIDCard, "classical", "key-auth-000001"), nil)
	assertCode(t, w, 40101)

	// 非法 token → 40101。
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID), submitBody(routerValidIDCard, "classical", "key-auth-000001"),
		map[string]string{"Authorization": "Bearer garbage"})
	assertCode(t, w, 40101)
}

func TestSubmitRegistrationSuccess(t *testing.T) {
	engine, signer, db := newRegistrationTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "报名成功赛事")
	userID := mustInsertRouterUser(t, db, "o-router-submit")
	userToken := userTokenFor(t, signer, userID)

	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		submitBody(routerValidIDCard, "classical", "key-ok-000000001"),
		map[string]string{"Authorization": "Bearer " + userToken})
	if w.Code != http.StatusOK {
		t.Fatalf("submit status = %d, body = %s", w.Code, w.Body)
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
	// snake_case 键断言（防 PascalCase 假绿）与字段收敛。
	assertSnakeCaseKeys(t, "submit registration", raw.Data)
	assertObjectKeys(t, "submit registration", raw.Data,
		"order_no", "registration_id", "amount", "level", "dance_type", "material_deadline")
	if len(raw.Data["order_no"].(string)) != 22 {
		t.Fatalf("order_no = %v, want 22 digits", raw.Data["order_no"])
	}
	if raw.Data["level"] != "U11" || raw.Data["dance_type"] != "classical" || raw.Data["amount"] != float64(98000) {
		t.Fatalf("resp data = %v", raw.Data)
	}
	if raw.Data["material_deadline"] == nil {
		t.Fatalf("material_deadline missing: %v", raw.Data)
	}
	// 响应中不得出现证件号与内部字段。
	for _, k := range []string{"id_card_no", "player_id", "group_id", "request_id", "score_rules"} {
		if _, ok := raw.Data[k]; ok {
			t.Fatalf("internal key %q exposed: %v", k, raw.Data)
		}
	}
}

func TestSubmitRegistrationRateLimit(t *testing.T) {
	engine, signer, db := newRegistrationTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "限流赛事")
	userID := mustInsertRouterUser(t, db, "o-router-limit")
	userToken := userTokenFor(t, signer, userID)
	headers := map[string]string{"Authorization": "Bearer " + userToken}

	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		submitBody(routerValidIDCard, "classical", "key-limit-0000001"), headers)
	if w.Code != http.StatusOK {
		t.Fatalf("first submit status = %d, body = %s", w.Code, w.Body)
	}
	// 同一用户 5 秒内第二次提交 → 42901（限流优先于业务校验）。
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		submitBody(routerValidIDCard, "folk", "key-limit-0000002"), headers)
	assertCode(t, w, 42901)
}

func TestSubmitRegistrationQuotaFullViaHTTP(t *testing.T) {
	engine, signer, db := newRegistrationTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)

	// 创建名额为 1 的发布赛事。
	body := fmt.Sprintf(`{
		"name": "单名额赛事", "season_id": %d, "level": "grade_a",
		"city": "上海", "venue": "场馆", "event_date": "2027-03-15",
		"fee": 98000, "publish": true,
		"timeline": {
			"reg_start_at": %q, "reg_end_at": %q, "lottery_at": %q,
			"material_check_at": %q, "start_at": %q
		},
		"groups": [{"level": "U11", "dance_type": "classical", "quota": 1}]
	}`, seasonID, offsetTime(-1), offsetTime(24), offsetTime(48), offsetTime(72), offsetTime(96))
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/events", body, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("create event status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			EventID int64 `json:"event_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	eventID := created.Data.EventID
	var groupID int64
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM event_groups WHERE event_id = ? AND level = 'U11' AND dance_type = 'classical'`,
		eventID).Scan(&groupID); err != nil {
		t.Fatalf("query group: %v", err)
	}

	// 选手 A（2016 出生）占掉唯一名额。
	userA := mustInsertRouterUser(t, db, "o-router-quota-a")
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		submitBody(routerValidIDCard, "classical", "key-quota-000001"),
		map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userA)})
	if w.Code != http.StatusOK {
		t.Fatalf("first submit status = %d, body = %s", w.Code, w.Body)
	}

	// 选手 B（2017 出生，同属 U11）→ 41005，data 附已满组别信息。
	// 110105201706150013 为手工核算校验位的合法身份证（GB 11643-1999）。
	userB := mustInsertRouterUser(t, db, "o-router-quota-b")
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		submitBody("110105201706150013", "classical", "key-quota-000002"),
		map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userB)})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("quota full status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			GroupID   int64  `json:"group_id"`
			Level     string `json:"level"`
			DanceType string `json:"dance_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != 41005 {
		t.Fatalf("code = %d, want 41005, body = %s", resp.Code, w.Body)
	}
	if resp.Data.GroupID != groupID || resp.Data.Level != "U11" || resp.Data.DanceType != "classical" {
		t.Fatalf("quota full data = %+v", resp.Data)
	}
}

func TestSubmitRegistrationBadRequest(t *testing.T) {
	engine, signer, db := newRegistrationTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "参数赛事")
	// 每个请求换用户：报名接口按用户限流 1 次/5 秒，避免 42901 干扰参数校验断言。
	headersOf := func(n int) map[string]string {
		userID := mustInsertRouterUser(t, db, fmt.Sprintf("o-router-badreq-%d", n))
		return map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}
	}

	// 非数字赛事 ID → 40001。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/events/abc/registrations",
		submitBody(routerValidIDCard, "classical", "key-badreq-00001"), headersOf(1))
	assertCode(t, w, 40001)

	// 缺必填字段 → 40001。
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		`{"player_name": "王小舞"}`, headersOf(2))
	assertCode(t, w, 40001)

	// 赛事不存在 → 40401。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/events/9999/registrations",
		submitBody(routerValidIDCard, "classical", "key-badreq-00002"), headersOf(3))
	assertCode(t, w, 40401)
}
