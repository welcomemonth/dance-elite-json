package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// mustInsertGroup 通过 store 在事务内创建报名单元，返回组别 ID。
func mustInsertGroup(t *testing.T, stores *Store, eventID int64, level, danceType string, quota int) int64 {
	t.Helper()
	var id int64
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = stores.Groups.Insert(context.Background(), tx, EventGroupParams{
			EventID: eventID, Level: level, DanceType: danceType,
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: quota,
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}
	return id
}

// TestTakeSlotTx 断言原子抢名额语义：抢到 taken+1；满员/关闭时返回行快照且不占名额。
func TestTakeSlotTx(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "抢名额赛事", "registering")

	// 正常抢到：taken 0 → 1。
	groupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeClassical, 2)
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		took, _, err := stores.Groups.TakeSlotTx(ctx, tx, groupID)
		if err != nil {
			return err
		}
		if !took {
			t.Fatal("first slot take should succeed")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	g, err := stores.Groups.Get(ctx, groupID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if g.Taken != 1 {
		t.Fatalf("taken = %d, want 1", g.Taken)
	}

	// 满员（quota=2，再抢一格后）→ 第三次抢不到，快照反映满员状态。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		took, _, err := stores.Groups.TakeSlotTx(ctx, tx, groupID)
		if err != nil || !took {
			t.Fatalf("second slot take = %v, %v", took, err)
		}
		took, snap, err := stores.Groups.TakeSlotTx(ctx, tx, groupID)
		if err != nil {
			return err
		}
		if took {
			t.Fatal("third slot take should miss when full")
		}
		if snap.Taken != 2 || snap.Quota != 2 || !snap.Enabled {
			t.Fatalf("full snapshot = %+v", snap)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	// 关闭组别 → 抢不到且快照 enabled=false；taken 不增长。
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.SetEnabled(ctx, tx, groupID, false)
	}); err != nil {
		t.Fatalf("disable group: %v", err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		took, snap, err := stores.Groups.TakeSlotTx(ctx, tx, groupID)
		if err != nil {
			return err
		}
		if took || snap.Enabled {
			t.Fatalf("disabled group take = %v, %+v", took, snap)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	if g, _ := stores.Groups.Get(ctx, groupID); g.Taken != 2 {
		t.Fatalf("taken = %d, want 2（关闭后不占名额）", g.Taken)
	}
}

// TestReleaseSlotTx 断言名额释放语义：taken 递减到 0；下溢时返回 ErrConflict。
func TestReleaseSlotTx(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "释放名额赛事", "registering")
	groupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeClassical, 2)

	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		if took, _, err := stores.Groups.TakeSlotTx(ctx, tx, groupID); err != nil || !took {
			t.Fatalf("take = %v, %v", took, err)
		}
		if took, _, err := stores.Groups.TakeSlotTx(ctx, tx, groupID); err != nil || !took {
			t.Fatalf("take = %v, %v", took, err)
		}
		return stores.Groups.ReleaseSlotTx(ctx, tx, groupID)
	})
	if err != nil {
		t.Fatalf("ReleaseSlotTx: %v", err)
	}
	if g, err := stores.Groups.Get(ctx, groupID); err != nil || g.Taken != 1 {
		t.Fatalf("taken = %+v, %v; want 1", g, err)
	}

	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.ReleaseSlotTx(ctx, tx, groupID)
	}); err != nil {
		t.Fatalf("second ReleaseSlotTx: %v", err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.ReleaseSlotTx(ctx, tx, groupID)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("release below zero err = %v, want ErrConflict", err)
	}
}

// TestOrderCancelTxAndHistory 覆盖订单取消条件流转、按 id 事务内查询与
// 历史订单列表（含已取消、不含待支付）。
func TestOrderCancelTxAndHistory(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "历史订单赛事", "registering")
	groupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeClassical, 2)
	folkGroupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeFolk, 2)
	userID := mustInsertUser(t, stores.db.sql, "o-order-cancel-store")
	playerID := mustInsertPlayer(t, stores.db.sql, "a8K2mP9z", "赵小舞", "hash-cancel-1")

	registrationID := mustInsertRegistration(t, stores.db.sql, eventID, groupID, playerID, userID,
		model.RegistrationStatusPendingPayment)
	var orderID int64
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		orderID, err = stores.Orders.CreateTx(ctx, tx, OrderParams{
			OrderNo: "2027010109300012345690", RegistrationID: registrationID, UserID: userID,
			Amount: 98000, Status: model.OrderStatusPendingPayment,
		})
		return err
	}); err != nil {
		t.Fatalf("create order: %v", err)
	}

	// 事务内按 id 查询。
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		got, err := stores.Orders.GetTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if got.ID != orderID || got.Status != model.OrderStatusPendingPayment {
			t.Fatalf("order = %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("GetTx: %v", err)
	}
	if _, err := stores.Orders.Get(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing order err = %v, want ErrNotFound", err)
	}

	const canceledAt = "2027-01-01T11:00:00+08:00"
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Orders.CancelTx(ctx, tx, orderID, model.OrderCancelReasonUserCancel, canceledAt)
	}); err != nil {
		t.Fatalf("CancelTx: %v", err)
	}
	canceled, err := stores.Orders.Get(ctx, orderID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if canceled.Status != model.OrderStatusCanceled || canceled.CanceledAt == nil ||
		*canceled.CanceledAt != canceledAt || canceled.CancelReason == nil ||
		*canceled.CancelReason != model.OrderCancelReasonUserCancel {
		t.Fatalf("canceled order = %+v", canceled)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Orders.CancelTx(ctx, tx, orderID, model.OrderCancelReasonUserCancel, canceledAt)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second CancelTx err = %v, want ErrConflict", err)
	}

	// 另一舞种的待支付订单：不进入历史列表。
	pendingRegID := mustInsertRegistration(t, stores.db.sql, eventID, folkGroupID, playerID, userID,
		model.RegistrationStatusPendingPayment)
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := stores.Orders.CreateTx(ctx, tx, OrderParams{
			OrderNo: "2027010109300012345691", RegistrationID: pendingRegID, UserID: userID,
			Amount: 98000, Status: model.OrderStatusPendingPayment,
		})
		return err
	}); err != nil {
		t.Fatalf("create pending order: %v", err)
	}

	rows, total, err := stores.Orders.ListHistoryByUser(ctx, userID, 1, 20)
	if err != nil {
		t.Fatalf("ListHistoryByUser: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("history = %d/%d, want 1/1（待支付不进入历史）", total, len(rows))
	}
	row := rows[0]
	if row.OrderID != orderID || row.Status != model.OrderStatusCanceled ||
		row.EventID != eventID || row.EventName != "历史订单赛事" ||
		row.Level != model.GroupLevelU11 || row.DanceType != model.DanceTypeClassical ||
		row.PlayerName != "赵小舞" || row.Amount != 98000 || row.CancelReason == nil {
		t.Fatalf("history row = %+v", row)
	}

	// 分页越界返回空列表，总数不变。
	empty, total, err := stores.Orders.ListHistoryByUser(ctx, userID, 2, 20)
	if err != nil {
		t.Fatalf("ListHistoryByUser page 2: %v", err)
	}
	if total != 1 || len(empty) != 0 {
		t.Fatalf("page 2 = %d/%d", total, len(empty))
	}
}
func TestOrderCreateAndIdempotentGet(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "订单赛事", "registering")
	groupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeClassical, 40)
	userID := mustInsertUser(t, stores.db.sql, "o-order-test")
	playerID := mustInsertPlayer(t, stores.db.sql, "a8K2mP9x", "王小舞", "hash-1")
	registrationID := mustInsertRegistration(t, stores.db.sql, eventID, groupID, playerID, userID, model.RegistrationStatusPendingPayment)

	var orderID int64
	requestID := "key-order-test-01"
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		orderID, err = stores.Orders.CreateTx(ctx, tx, OrderParams{
			OrderNo:        "2027010109300012345678",
			RegistrationID: registrationID,
			UserID:         userID,
			Amount:         98000,
			Status:         model.OrderStatusPendingPayment,
			RequestID:      &requestID,
		})
		return err
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if orderID <= 0 {
		t.Fatalf("order id = %d", orderID)
	}

	// 幂等查询命中：返回订单 + 组别 level/dance_type + 资料截止时间。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		got, err := stores.Orders.GetByRequestTx(ctx, tx, userID, requestID)
		if err != nil {
			return err
		}
		if got == nil {
			t.Fatal("idempotent get should hit")
		}
		if got.Order.OrderNo != "2027010109300012345678" || got.Order.Amount != 98000 ||
			got.Order.Status != model.OrderStatusPendingPayment || *got.Order.RequestID != requestID {
			t.Fatalf("order = %+v", got.Order)
		}
		if got.Level != model.GroupLevelU11 || got.DanceType != model.DanceTypeClassical {
			t.Fatalf("group context = %s/%s", got.Level, got.DanceType)
		}
		if got.MaterialCheckAt == nil || *got.MaterialCheckAt != "2027-03-01T18:00:00+08:00" {
			t.Fatalf("material_check_at = %v", got.MaterialCheckAt)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	// 未命中 → (nil, nil)。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		got, err := stores.Orders.GetByRequestTx(ctx, tx, userID, "no-such-key-0001")
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatalf("miss should return nil, got %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	// 同人同 request_id 再插入 → 唯一冲突（service 层映射 40901）。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := stores.Orders.CreateTx(ctx, tx, OrderParams{
			OrderNo: "2027010109300099999999", RegistrationID: registrationID, UserID: userID,
			Amount: 98000, Status: model.OrderStatusPendingPayment, RequestID: &requestID,
		})
		if !IsUniqueViolation(err) {
			t.Fatalf("duplicate request_id err = %v, want unique violation", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
