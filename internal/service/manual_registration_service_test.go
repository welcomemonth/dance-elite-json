package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/store"
)

// manualFixture 在报名测试装配之上挂载人工报名服务。
type manualFixture struct {
	*regFixture
	svc     *ManualRegistrationService
	adminID int64
}

func newManualFixture(t *testing.T) *manualFixture {
	t.Helper()
	f := newRegFixture(t)
	var adminID int64
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM admins WHERE username = 'admin'`).Scan(&adminID); err != nil {
		t.Fatalf("query admin id: %v", err)
	}
	players := NewPlayerService(f.stores, "test-hmac-key")
	return &manualFixture{
		regFixture: f,
		svc:        NewManualRegistrationService(f.stores, players),
		adminID:    adminID,
	}
}

// manualEvent 创建报名中的赛事（U11 古典舞，指定名额），返回赛事/组别 ID。
func (f *manualFixture) manualEvent(t *testing.T, name string, quota int) (eventID, groupID int64) {
	t.Helper()
	eventID = mustCreateRegisteringEvent(t, f.regFixture, name, []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: quota},
	})
	groupID = mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)
	return eventID, groupID
}

// passportReq 构造护照人工报名请求（出生年份落在 U11：2016-2018）。
func passportReq(eventID, groupID int64, passport string) model.ManualRegistrationRequest {
	return model.ManualRegistrationRequest{
		EventID:      eventID,
		GroupID:      groupID,
		PlayerName:   "Emma Wilson",
		IDType:       "passport",
		IDCardNo:     passport,
		BirthDate:    "2017-05-01",
		Nationality:  "美国",
		Institution:  "国际舞蹈学校",
		ContactPhone: "13800000001",
		ChargeType:   model.ManualChargeOfflinePaid,
		Amount:       98000,
		Remark:       "特殊证件登记",
	}
}

// TestManualRegistrationCreate 覆盖护照建档、已支付订单/流水、名额与审计留痕。
func TestManualRegistrationCreate(t *testing.T) {
	f := newManualFixture(t)
	eventID, groupID := f.manualEvent(t, "人工报名赛事", 5)
	ctx := context.Background()

	csSvc := NewCSTicketService(f.stores)
	ticket, err := csSvc.Create(ctx, f.adminID, model.CreateCSTicketRequest{
		Type: model.CSTicketTypeNoIDCard, Phone: "13800000001", Content: "外籍选手报名",
	})
	if err != nil {
		t.Fatalf("create cs ticket: %v", err)
	}
	req := passportReq(eventID, groupID, " P1234567 ")
	req.CSTicketID = ticket.TicketID

	resp, err := f.svc.Create(ctx, f.adminID, req)
	if err != nil {
		t.Fatalf("create manual registration: %v", err)
	}
	if resp.RegistrationID <= 0 || resp.OrderNo == "" || len(resp.PlayerNo) != 8 {
		t.Fatalf("resp = %+v", resp)
	}

	var status, source, phone, specialReq string
	var playerID, userID int64
	var csTicketID sql.NullInt64
	if err := f.db.SQL().QueryRowContext(ctx, `
		SELECT status, source, contact_phone, special_req, player_id, user_id, cs_ticket_id
		FROM registrations WHERE id = ?`, resp.RegistrationID).Scan(
		&status, &source, &phone, &specialReq, &playerID, &userID, &csTicketID); err != nil {
		t.Fatalf("query registration: %v", err)
	}
	if status != model.RegistrationStatusActive || source != model.OrderSourceManual ||
		phone != "13800000001" || specialReq != "特殊证件登记" {
		t.Fatalf("registration = %s/%s/%s/%s", status, source, phone, specialReq)
	}
	if !csTicketID.Valid || csTicketID.Int64 != ticket.TicketID {
		t.Fatalf("cs_ticket_id = %v, want %d", csTicketID, ticket.TicketID)
	}

	var orderStatus, orderSource string
	var amount int64
	var paidAt sql.NullString
	if err := f.db.SQL().QueryRowContext(ctx, `
		SELECT status, source, amount, paid_at FROM orders WHERE order_no = ?`, resp.OrderNo).Scan(
		&orderStatus, &orderSource, &amount, &paidAt); err != nil {
		t.Fatalf("query order: %v", err)
	}
	if orderStatus != model.OrderStatusPaid || orderSource != model.OrderSourceManual ||
		amount != 98000 || !paidAt.Valid || paidAt.String == "" {
		t.Fatalf("order = %s/%s/%d/%v", orderStatus, orderSource, amount, paidAt)
	}

	var payType, payStatus, raw string
	var payAmount int64
	if err := f.db.SQL().QueryRowContext(ctx, `
		SELECT type, status, amount, raw_callback FROM payments
		WHERE order_id = (SELECT id FROM orders WHERE order_no = ?)`, resp.OrderNo).Scan(
		&payType, &payStatus, &payAmount, &raw); err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if payType != model.PaymentTypePay || payStatus != model.PaymentStatusSuccess || payAmount != 98000 {
		t.Fatalf("payment = %s/%s/%d", payType, payStatus, payAmount)
	}
	if !strings.Contains(raw, "后台人工录入") {
		t.Fatalf("raw_callback = %q, want 人工备注", raw)
	}

	var playerIDType, birthDate, nationality string
	if err := f.db.SQL().QueryRowContext(ctx, `
		SELECT id_type, birth_date, nationality FROM players WHERE id = ?`, playerID).Scan(
		&playerIDType, &birthDate, &nationality); err != nil {
		t.Fatalf("query player: %v", err)
	}
	if playerIDType != "passport" || birthDate != "2017-05-01" || nationality != "美国" {
		t.Fatalf("player = %s/%s/%s", playerIDType, birthDate, nationality)
	}
	if userID <= 0 {
		t.Fatal("manual registration should bind the reserved manual user")
	}

	if taken := groupTaken(t, f.db, groupID); taken != 1 {
		t.Fatalf("taken = %d, want 1", taken)
	}
	if n := countAdminLogOf(t, f.db, "admin.manual_registration.create", "registration", resp.RegistrationID); n != 1 {
		t.Fatalf("admin logs = %d, want 1", n)
	}
	var ticketReg sql.NullInt64
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT related_registration_id FROM cs_tickets WHERE id = ?`, ticket.TicketID).Scan(&ticketReg); err != nil {
		t.Fatalf("query cs ticket: %v", err)
	}
	if !ticketReg.Valid || ticketReg.Int64 != resp.RegistrationID {
		t.Fatalf("cs ticket related_registration_id = %v", ticketReg)
	}

	// 同护照同组别重复报名 → 41004。
	if _, err := f.svc.Create(ctx, f.adminID, passportReq(eventID, groupID, "P1234567")); mustBizCode(t, err) != 41004 {
		t.Fatalf("duplicate should be 41004, got %v", err)
	}
	// 同一工单再次关联 → 40901。
	otherEventID, otherGroupID := f.manualEvent(t, "人工报名赛事二", 5)
	second := passportReq(otherEventID, otherGroupID, "P7654321")
	second.CSTicketID = ticket.TicketID
	if _, err := f.svc.Create(ctx, f.adminID, second); mustBizCode(t, err) != 40901 {
		t.Fatalf("linked ticket should be 40901, got %v", err)
	}
}

// TestManualRegistrationValidation 覆盖年龄、名额、赛事状态、收费方式与工单校验。
func TestManualRegistrationValidation(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()

	// 超龄（U11 要求 2016-2018）→ 41002。
	eventID, groupID := f.manualEvent(t, "人工报名校验", 5)
	overage := passportReq(eventID, groupID, "P-OVERAGE")
	overage.BirthDate = "2000-01-01"
	if _, err := f.svc.Create(ctx, f.adminID, overage); mustBizCode(t, err) != 41002 {
		t.Fatalf("overage should be 41002, got %v", err)
	}

	// 免费通道：金额固定 0。
	free := passportReq(eventID, groupID, "P-FREE")
	free.ChargeType = model.ManualChargeFree
	free.Amount = 50000
	resp, err := f.svc.Create(ctx, f.adminID, free)
	if err != nil {
		t.Fatalf("free manual registration: %v", err)
	}
	var amount int64
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT amount FROM orders WHERE order_no = ?`, resp.OrderNo).Scan(&amount); err != nil {
		t.Fatalf("query free order: %v", err)
	}
	if amount != 0 {
		t.Fatalf("free order amount = %d, want 0", amount)
	}

	// 名额已满（quota=1 已被免费报名占用）→ 41005。
	fullEvent, fullGroup := f.manualEvent(t, "人工报名满员", 1)
	if _, err := f.svc.Create(ctx, f.adminID, passportReq(fullEvent, fullGroup, "P-FIRST")); err != nil {
		t.Fatalf("fill quota: %v", err)
	}
	if _, err := f.svc.Create(ctx, f.adminID, passportReq(fullEvent, fullGroup, "P-SECOND")); mustBizCode(t, err) != 41005 {
		t.Fatalf("full quota should be 41005, got %v", err)
	}

	// 赛事不在报名期 → 41006。
	closedEvent, closedGroup := f.manualEvent(t, "人工报名截止", 5)
	mustExecSQL(t, f.db,
		`UPDATE events SET reg_start_at = '2019-12-01T00:00:00+08:00',
		 reg_end_at = '2020-01-01T00:00:00+08:00' WHERE id = ?`, closedEvent)
	if _, err := f.svc.Create(ctx, f.adminID, passportReq(closedEvent, closedGroup, "P-CLOSED")); mustBizCode(t, err) != 41006 {
		t.Fatalf("closed event should be 41006, got %v", err)
	}

	// 收费方式/金额/证件类型/工单校验 → 40001 / 40401。
	cases := []struct {
		name string
		mod  func(*model.ManualRegistrationRequest)
		code int
	}{
		{"收费方式非法", func(r *model.ManualRegistrationRequest) { r.ChargeType = "bogus" }, 40001},
		{"线下收款金额为 0", func(r *model.ManualRegistrationRequest) { r.Amount = 0 }, 40001},
		{"证件类型非法", func(r *model.ManualRegistrationRequest) { r.IDType = "bogus" }, 40001},
		{"缺出生日期", func(r *model.ManualRegistrationRequest) { r.BirthDate = "" }, 40001},
		{"缺联系电话", func(r *model.ManualRegistrationRequest) { r.ContactPhone = "" }, 40001},
		{"工单不存在", func(r *model.ManualRegistrationRequest) { r.CSTicketID = 999999 }, 40401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := passportReq(eventID, groupID, "P-CASE")
			tc.mod(&req)
			if _, err := f.svc.Create(ctx, f.adminID, req); mustBizCode(t, err) != tc.code {
				t.Fatalf("code = %v, want %d", err, tc.code)
			}
		})
	}
}

// TestManualRegistrationIDTypeAutoDetect 覆盖 id_type 缺省时按身份证格式自动判定。
func TestManualRegistrationIDTypeAutoDetect(t *testing.T) {
	f := newManualFixture(t)
	eventID, groupID := f.manualEvent(t, "人工报名自动判定", 5)
	req := passportReq(eventID, groupID, validIDForBirth("20170501"))
	req.IDType = ""
	req.BirthDate = ""

	resp, err := f.svc.Create(context.Background(), f.adminID, req)
	if err != nil {
		t.Fatalf("auto detect create: %v", err)
	}
	var idType, birthDate string
	if err := f.db.SQL().QueryRowContext(context.Background(), `
		SELECT p.id_type, p.birth_date FROM players p
		JOIN registrations r ON r.player_id = p.id
		WHERE r.id = ?`, resp.RegistrationID).Scan(&idType, &birthDate); err != nil {
		t.Fatalf("query player: %v", err)
	}
	if idType != "id_card" || birthDate != "2017-05-01" {
		t.Fatalf("player = %s/%s, want id_card/2017-05-01", idType, birthDate)
	}
}

// TestPassportAndIDCardArchivesSeparate 护照号与身份证分别建档、互不冲突。
func TestPassportAndIDCardArchivesSeparate(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	eventID, groupID := f.manualEvent(t, "双证件建档", 5)

	passport := passportReq(eventID, groupID, "E12345678")
	passportResp, err := f.svc.Create(ctx, f.adminID, passport)
	if err != nil {
		t.Fatalf("passport registration: %v", err)
	}

	idCard := passportReq(eventID, groupID, validIDForBirth("20170501"))
	idCard.IDType = ""
	idCard.BirthDate = ""
	idCard.ContactPhone = "13800000009"
	idResp, err := f.svc.Create(ctx, f.adminID, idCard)
	if err != nil {
		t.Fatalf("id card registration: %v", err)
	}
	if passportResp.PlayerNo == idResp.PlayerNo {
		t.Fatal("passport and id_card should create separate players")
	}

	rows, err := f.db.SQL().QueryContext(ctx, `
		SELECT p.id_type FROM players p
		JOIN registrations r ON r.player_id = p.id
		WHERE r.id IN (?, ?) ORDER BY p.id_type`, passportResp.RegistrationID, idResp.RegistrationID)
	if err != nil {
		t.Fatalf("query id types: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var idType string
		if err := rows.Scan(&idType); err != nil {
			t.Fatalf("scan id type: %v", err)
		}
		types = append(types, idType)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate id types: %v", err)
	}
	if len(types) != 2 || types[0] != "id_card" || types[1] != "passport" {
		t.Fatalf("id types = %v, want [id_card passport]", types)
	}
}

// countAdminLogOf 统计指定动作、目标类型与目标 ID 的审计日志条数。
func countAdminLogOf(t *testing.T, db *store.DB, action, targetType string, targetID int64) int {
	t.Helper()
	var n int
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(1) FROM admin_logs WHERE action = ? AND target_type = ? AND target_id = ?`,
		action, targetType, targetID).Scan(&n); err != nil {
		t.Fatalf("count admin log: %v", err)
	}
	return n
}
