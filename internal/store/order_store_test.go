package store

import (
	"context"
	"database/sql"
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

// TestOrderCreateAndIdempotentGet 覆盖订单创建与 (user_id, request_id) 幂等查询。
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
