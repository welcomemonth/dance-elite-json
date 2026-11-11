package service

import (
	"context"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/store"
)

// waitlistFixture 在报名测试装配之上挂载候补服务。
type waitlistFixture struct {
	*regFixture
	svc *WaitlistService
}

func newWaitlistFixture(t *testing.T) *waitlistFixture {
	t.Helper()
	f := newRegFixture(t)
	return &waitlistFixture{
		regFixture: f,
		svc:        NewWaitlistService(f.stores, NewPlayerService(f.stores, "test-hmac-key")),
	}
}

// createFullGroupEvent 创建并发布只含一个 U11 古典舞组别的赛事，并把名额占满，
// 返回赛事 ID 与组别 ID（用于触发名额已满的候补流程）。
func (f *waitlistFixture) createFullGroupEvent(t *testing.T, name string) (eventID, groupID int64) {
	t.Helper()
	eventID = mustCreateRegisteringEvent(t, f.regFixture, name, []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 1},
	})
	groupID = mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)
	mustExecSQL(t, f.db, `UPDATE event_groups SET taken = quota WHERE id = ?`, groupID)
	return eventID, groupID
}

// mustExecSQL 执行原始 SQL（测试构造满员/停用等状态）。
func mustExecSQL(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.SQL().ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// waitlistReq 构造候补提交请求。
func waitlistReq(groupID int64, name, idCard, phone string) model.SubmitWaitlistRequest {
	return model.SubmitWaitlistRequest{
		GroupID:  groupID,
		Name:     name,
		IDCardNo: idCard,
		Phone:    phone,
	}
}

// mustOrder 为指定用户与报名单元构造一条 active 报名 + 指定状态的订单，返回订单号。
func mustOrder(t *testing.T, db *store.DB, eventID, groupID, userID int64, playerNo, orderNo, status string) string {
	t.Helper()
	ctx := context.Background()
	res, err := db.SQL().ExecContext(ctx,
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, '关联选手', ?)`,
		playerNo, "hash-"+playerNo)
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
	var paidAt any
	if status == model.OrderStatusPaid {
		paidAt = "2027-01-01T10:00:00+08:00"
	}
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO orders (order_no, registration_id, user_id, amount, status, paid_at)
		 VALUES (?, ?, ?, 98000, ?, ?)`, orderNo, regID, userID, status, paidAt); err != nil {
		t.Fatalf("insert order: %v", err)
	}
	return orderNo
}
