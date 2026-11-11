package router

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/meetmonth/dance-elite/internal/store"
)

// mustHTTPOrder 插入一条指定状态/来源的订单及 active 报名，返回订单号。
func mustHTTPOrder(t *testing.T, db *store.DB, eventID, groupID, userID int64,
	playerNo, orderNo, status, source string) {
	t.Helper()
	ctx := context.Background()
	res, err := db.SQL().ExecContext(ctx,
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, '后台查询选手', ?)`,
		playerNo, "hash-"+playerNo)
	if err != nil {
		t.Fatalf("insert player %s: %v", playerNo, err)
	}
	playerID, _ := res.LastInsertId()
	res, err = db.SQL().ExecContext(ctx,
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status, source)
		 VALUES (?, ?, ?, ?, 'classical', 'active', ?)`, eventID, groupID, playerID, userID, source)
	if err != nil {
		t.Fatalf("insert registration %s: %v", orderNo, err)
	}
	regID, _ := res.LastInsertId()
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO orders (order_no, registration_id, user_id, amount, status, source, paid_at)
		 VALUES (?, ?, ?, 98000, ?, ?, '2027-01-01T10:00:00+08:00')`,
		orderNo, regID, userID, status, source); err != nil {
		t.Fatalf("insert order %s: %v", orderNo, err)
	}
}

// decodeData 解码统一响应包装中的 data 对象。
func decodeData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var raw struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	return raw.Data
}

// TestAdminOrdersListHTTP 覆盖订单列表的鉴权、角色、筛选参数与字段收敛。
func TestAdminOrdersListHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "订单列表接口", 10)
	userID := mustInsertRouterUser(t, db, "o-admin-orders")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpPly01", "ORD-HTTP-1", "paid", "miniapp")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpPly02", "ORD-HTTP-2", "canceled", "miniapp")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpPly03", "ORD-HTTP-3", "paid", "manual")

	// 未登录 → 40101。
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders", "", nil), 40101)

	// cs 只读可用 → 200；默认排除已取消。
	w := postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders", "", csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("orders status = %d, body = %s", w.Code, w.Body)
	}
	data := decodeData(t, w.Body.Bytes())
	assertSnakeCaseKeys(t, "admin orders", data)
	if data["total"] != float64(2) {
		t.Fatalf("total = %v, want 2 (canceled excluded)", data["total"])
	}
	items, _ := data["list"].([]any)
	if len(items) != 2 {
		t.Fatalf("list len = %d", len(items))
	}
	item, _ := items[0].(map[string]any)
	assertSnakeCaseKeys(t, "admin order item", item)
	assertObjectKeys(t, "admin order item", item,
		"order_no", "transaction_id", "event_id", "event_name", "level", "dance_type",
		"player_no", "player_name", "id_type", "amount", "status", "source",
		"paid_at", "created_at")
	for _, forbidden := range []string{"id_card_no", "id_card_hash", "id_card_masked"} {
		if _, ok := item[forbidden]; ok {
			t.Fatalf("admin order must not expose %s: %v", forbidden, item)
		}
	}

	// include_canceled=true 含留档订单。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders?include_canceled=true", "", adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("include canceled status = %d, body = %s", w.Code, w.Body)
	}
	if total := decodeData(t, w.Body.Bytes())["total"]; total != float64(3) {
		t.Fatalf("total with canceled = %v, want 3", total)
	}

	// source=manual 筛选。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders?source=manual", "", adminHeaders)
	if total := decodeData(t, w.Body.Bytes())["total"]; total != float64(1) {
		t.Fatalf("manual total = %v, want 1", total)
	}

	// 非法筛选值 → 40001。
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders?status=bogus", "", adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders?level=U99", "", adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders?paid_from=bad", "", adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders?event_id=x", "", adminHeaders), 40001)
}

// TestAdminRegistrationsListHTTP 覆盖报名名单的资料完成度筛选与缺失项标注。
func TestAdminRegistrationsListHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "报名名单接口", 10)
	userID := mustInsertRouterUser(t, db, "o-admin-regs")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpReg01", "ORD-REG-1", "paid", "miniapp")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpReg02", "ORD-REG-2", "paid", "miniapp")

	// 第一条资料全部填齐；第二条全部为空。
	if _, err := db.SQL().ExecContext(context.Background(), `
		UPDATE registrations SET photo_path='p.jpg', audio_path='a.mp3',
			program_name='节目', teacher_name='老师', contact_phone='13800000001',
			express_address='地址', work_intro='简介', special_req='无'
		WHERE player_id = (SELECT id FROM players WHERE player_no = 'HtpReg01')`); err != nil {
		t.Fatalf("fill materials: %v", err)
	}

	// 未登录 → 40101；cs 只读可用。
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/registrations", "", nil), 40101)
	w := postJSON(t, engine, http.MethodGet, "/api/v1/admin/registrations?material_state=incomplete", "", csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("registrations status = %d, body = %s", w.Code, w.Body)
	}
	data := decodeData(t, w.Body.Bytes())
	assertSnakeCaseKeys(t, "admin registrations", data)
	if data["total"] != float64(1) {
		t.Fatalf("incomplete total = %v, want 1", data["total"])
	}
	items, _ := data["list"].([]any)
	item, _ := items[0].(map[string]any)
	assertSnakeCaseKeys(t, "admin registration item", item)
	assertObjectKeys(t, "admin registration item", item,
		"registration_id", "order_no", "event_id", "event_name", "group_id", "level",
		"dance_type", "player_no", "player_name", "id_type", "institution", "contact_phone",
		"status", "source", "material_completed", "material_total", "missing_items",
		"material_updated_at", "created_at")
	if item["material_completed"] != float64(0) || item["material_total"] != float64(8) {
		t.Fatalf("material = %v/%v", item["material_completed"], item["material_total"])
	}
	missing, _ := item["missing_items"].([]any)
	if len(missing) != 8 {
		t.Fatalf("missing_items = %v, want 8 items", missing)
	}

	// 完整名单可见（material_state=complete）。
	w = postJSON(t, engine, http.MethodGet, "/api/v1/admin/registrations?material_state=complete", "", adminHeaders)
	if total := decodeData(t, w.Body.Bytes())["total"]; total != float64(1) {
		t.Fatalf("complete total = %v, want 1", total)
	}

	// 非法筛选值 → 40001。
	assertCode(t, postJSON(t, engine, http.MethodGet,
		"/api/v1/admin/registrations?material_state=partial", "", adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodGet,
		"/api/v1/admin/registrations?missing_item=teacher_name", "", adminHeaders), 40001)
}

// TestAdminOrderDetailHTTP 覆盖订单详情的鉴权、关联区块与错误码。
func TestAdminOrderDetailHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "订单详情接口", 10)
	userID := mustInsertRouterUser(t, db, "o-admin-detail")
	mustHTTPOrder(t, db, eventID, groupID, userID, "HtpDtl01", "ORD-DTL-1", "paid", "miniapp")

	// 补齐支付流水、退款工单、客服工单。
	ctx := context.Background()
	if _, err := db.SQL().ExecContext(ctx, `
		INSERT INTO payments (order_id, type, transaction_id, amount, status, notified_at)
		VALUES ((SELECT id FROM orders WHERE order_no = 'ORD-DTL-1'), 'pay', '4200DTL', 98000,
		        'success', '2027-01-01T10:00:00+08:00')`); err != nil {
		t.Fatalf("insert payment: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx, `
		INSERT INTO refund_tickets (ticket_no, order_id, reason, status, created_by, created_at)
		VALUES ('RF-DTL-1', (SELECT id FROM orders WHERE order_no = 'ORD-DTL-1'),
		        '伤病退赛', 'pending', 1, '2027-01-02T09:00:00+08:00')`); err != nil {
		t.Fatalf("insert refund ticket: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx, `
		INSERT INTO cs_tickets (type, phone, content, related_order_id, status, created_by)
		VALUES ('refund', '13800000001', '申请退款',
		        (SELECT id FROM orders WHERE order_no = 'ORD-DTL-1'), 'pending', 1)`); err != nil {
		t.Fatalf("insert cs ticket: %v", err)
	}

	// 未登录 → 40101；cs 只读可用 → 200。
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders/ORD-DTL-1", "", nil), 40101)
	w := postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders/ORD-DTL-1", "", csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", w.Code, w.Body)
	}
	data := decodeData(t, w.Body.Bytes())
	assertSnakeCaseKeys(t, "admin order detail", data)
	assertObjectKeys(t, "admin order detail", data,
		"order", "registration", "payments", "refund_tickets", "cs_tickets",
		"materials", "material_status", "operation_logs")

	order, _ := data["order"].(map[string]any)
	assertSnakeCaseKeys(t, "admin order detail order", order)
	assertObjectKeys(t, "admin order detail order", order,
		"order_no", "transaction_id", "registration_id", "event_id", "event_name",
		"level", "dance_type", "player_no", "player_name", "id_type", "amount",
		"status", "source", "paid_at", "canceled_at", "cancel_reason", "created_at")
	for _, forbidden := range []string{"id_card_no", "id_card_hash", "id_card_masked"} {
		if _, ok := order[forbidden]; ok {
			t.Fatalf("order detail must not expose %s", forbidden)
		}
	}
	if order["order_no"] != "ORD-DTL-1" || order["transaction_id"] != "4200DTL" ||
		order["player_no"] != "HtpDtl01" {
		t.Fatalf("order = %v", order)
	}
	if payments, _ := data["payments"].([]any); len(payments) != 1 {
		t.Fatalf("payments = %v", data["payments"])
	}
	if tickets, _ := data["refund_tickets"].([]any); len(tickets) != 1 {
		t.Fatalf("refund tickets = %v", data["refund_tickets"])
	}
	if tickets, _ := data["cs_tickets"].([]any); len(tickets) != 1 {
		t.Fatalf("cs tickets = %v", data["cs_tickets"])
	}

	// 不存在 → 40401。
	assertCode(t, postJSON(t, engine, http.MethodGet, "/api/v1/admin/orders/ORD-NOPE", "", adminHeaders), 40401)
}
