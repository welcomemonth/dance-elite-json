package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// adminQueryFixture 在报名测试装配之上挂载后台查询服务，并提供订单/报名种子。
type adminQueryFixture struct {
	*regFixture
	orderSvc *AdminOrderQueryService
	regSvc   *AdminRegistrationQueryService
	eventID  int64
	seq      int
}

// adminSeed 是一条种子报名（订单 + 报名单 + 选手 + 用户）。
type adminSeed struct {
	OrderNo        string
	RegistrationID int64
	PlayerID       int64
	UserID         int64
	PlayerNo       string
}

// newAdminQueryFixture 创建含 U11 古典舞与 U13 民族民间舞两个报名单元的赛事。
func newAdminQueryFixture(t *testing.T) *adminQueryFixture {
	t.Helper()
	f := newRegFixture(t)
	eventID := mustCreateRegisteringEvent(t, f, "后台查询赛事", []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 20},
		{Level: model.GroupLevelU13, DanceType: model.DanceTypeFolk, Quota: 20},
	})
	return &adminQueryFixture{
		regFixture: f,
		orderSvc:   NewAdminOrderQueryService(f.stores),
		regSvc:     NewAdminRegistrationQueryService(f.stores),
		eventID:    eventID,
	}
}

// seed 插入一条选手/报名单/订单并返回 ID 汇总；报名状态随订单状态推导。
func (f *adminQueryFixture) seed(t *testing.T, tag, orderNo, status, source, danceType string, groupID int64, playerName string) adminSeed {
	t.Helper()
	ctx := context.Background()
	f.seq++
	playerNo := fmt.Sprintf("Q%07d", f.seq)
	res, err := f.db.SQL().ExecContext(ctx,
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, ?, ?)`,
		playerNo, playerName, "hash-"+playerNo)
	if err != nil {
		t.Fatalf("insert player %s: %v", tag, err)
	}
	playerID, _ := res.LastInsertId()

	regStatus := model.RegistrationStatusActive
	if status == model.OrderStatusCanceled {
		regStatus = model.RegistrationStatusCanceled
	}
	if status == model.OrderStatusPendingPayment {
		regStatus = model.RegistrationStatusPendingPayment
	}
	userID := mustInsertRegUser(t, f.db, "o-admin-query-"+tag)
	res, err = f.db.SQL().ExecContext(ctx,
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.eventID, groupID, playerID, userID, danceType, regStatus, source)
	if err != nil {
		t.Fatalf("insert registration %s: %v", tag, err)
	}
	regID, _ := res.LastInsertId()

	var paidAt any
	if status != model.OrderStatusPendingPayment && status != model.OrderStatusCanceled {
		paidAt = "2027-01-01T10:00:00+08:00"
	}
	if _, err := f.db.SQL().ExecContext(ctx,
		`INSERT INTO orders (order_no, registration_id, user_id, amount, status, source, paid_at)
		 VALUES (?, ?, ?, 98000, ?, ?, ?)`,
		orderNo, regID, userID, status, source, paidAt); err != nil {
		t.Fatalf("insert order %s: %v", tag, err)
	}
	return adminSeed{OrderNo: orderNo, RegistrationID: regID, PlayerID: playerID, UserID: userID, PlayerNo: playerNo}
}

// setPaidAt 覆盖订单支付时间（支付时间范围筛选测试用）。
func (f *adminQueryFixture) setPaidAt(t *testing.T, orderNo, paidAt string) {
	t.Helper()
	mustExecSQL(t, f.db, `UPDATE orders SET paid_at = ? WHERE order_no = ?`, paidAt, orderNo)
}

// addPayPayment 写入一条成功支付流水（微信支付单号关键字测试用）。
func (f *adminQueryFixture) addPayPayment(t *testing.T, orderNo, transactionID string) {
	t.Helper()
	mustExecSQL(t, f.db, `
		INSERT INTO payments (order_id, type, transaction_id, amount, status, notified_at)
		VALUES ((SELECT id FROM orders WHERE order_no = ?), 'pay', ?, 98000, 'success',
		        '2027-01-01T10:00:00+08:00')`, orderNo, transactionID)
}

// fillMaterials 覆盖报名单资料字段：fields 中列出的项写入测试值，其余置 NULL。
func (f *adminQueryFixture) fillMaterials(t *testing.T, regID int64, fields ...string) {
	t.Helper()
	set := make(map[string]bool, len(fields))
	for _, field := range fields {
		set[field] = true
	}
	value := func(column string) any {
		if set[column] {
			return "v-" + column
		}
		return nil
	}
	mustExecSQL(t, f.db, `UPDATE registrations SET
		photo_path = ?, audio_path = ?, program_name = ?, teacher_name = ?,
		contact_phone = ?, express_address = ?, work_intro = ?, special_req = ?,
		material_updated_at = '2027-01-02T09:00:00+08:00'
		WHERE id = ?`,
		value("photo"), value("audio"), value("program_name"), value("teacher_name"),
		value("contact_phone"), value("express_address"), value("work_intro"), value("special_req"),
		regID)
}

// setUserPhone 覆盖用户手机号（报名单联系电话回退测试用）。
func (f *adminQueryFixture) setUserPhone(t *testing.T, userID int64, phone string) {
	t.Helper()
	mustExecSQL(t, f.db, `UPDATE users SET phone = ? WHERE id = ?`, phone, userID)
}

// allMaterialFields 是资料完成度 8 项。
var allMaterialFields = []string{
	model.MaterialFieldPhoto, model.MaterialFieldAudio, model.MaterialFieldProgramName,
	model.MaterialFieldTeacherName, model.MaterialFieldContactPhone,
	model.MaterialFieldExpressAddress, model.MaterialFieldWorkIntro, model.MaterialFieldSpecialReq,
}
