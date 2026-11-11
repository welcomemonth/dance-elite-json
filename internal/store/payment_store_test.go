package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// mustPendingOrderFixture 建一场赛事、一条待支付报名单与一笔待支付订单。
// 返回订单 ID、报名单 ID 与订单号。
func mustPendingOrderFixture(t *testing.T, stores *Store, orderNo string) (int64, int64, string) {
	t.Helper()
	ctx := context.Background()
	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "支付流水赛事", "registering")
	groupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeClassical, 40)
	userID := mustInsertUser(t, stores.db.sql, "o-payment-store")
	playerID := mustInsertPlayer(t, stores.db.sql, "a8K2mP9y", "李小舞", "hash-pay-1")
	registrationID := mustInsertRegistration(t, stores.db.sql, eventID, groupID, playerID, userID,
		model.RegistrationStatusPendingPayment)

	var orderID int64
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		orderID, err = stores.Orders.CreateTx(ctx, tx, OrderParams{
			OrderNo: orderNo, RegistrationID: registrationID, UserID: userID,
			Amount: 98000, Status: model.OrderStatusPendingPayment,
		})
		return err
	}); err != nil {
		t.Fatalf("create order: %v", err)
	}
	return orderID, registrationID, orderNo
}

// TestOrderMarkPaidTx 覆盖按订单号查询与 pending_payment → paid 的条件流转。
func TestOrderMarkPaidTx(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	orderID, _, orderNo := mustPendingOrderFixture(t, stores, "2027010109300012345678")

	got, err := stores.Orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		t.Fatalf("GetByOrderNo: %v", err)
	}
	if got.ID != orderID || got.Status != model.OrderStatusPendingPayment {
		t.Fatalf("order = %+v", got)
	}
	if _, err := stores.Orders.GetByOrderNo(ctx, "no-such-order"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing order err = %v, want ErrNotFound", err)
	}

	const paidAt = "2027-01-01T10:00:00+08:00"
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Orders.MarkPaidTx(ctx, tx, orderID, paidAt)
	}); err != nil {
		t.Fatalf("MarkPaidTx: %v", err)
	}
	paid, err := stores.Orders.Get(ctx, orderID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if paid.Status != model.OrderStatusPaid || paid.PaidAt == nil || *paid.PaidAt != paidAt {
		t.Fatalf("paid order = %+v", paid)
	}

	// 非待支付再流转 → ErrConflict（幂等由 service 层判断）。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Orders.MarkPaidTx(ctx, tx, orderID, paidAt)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second MarkPaidTx err = %v, want ErrConflict", err)
	}
}

// TestRegistrationActivateTx 覆盖报名单 pending_payment → active 的条件流转。
func TestRegistrationActivateTx(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	_, registrationID, _ := mustPendingOrderFixture(t, stores, "2027010109300012345679")

	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Registrations.ActivateTx(ctx, tx, registrationID)
	}); err != nil {
		t.Fatalf("ActivateTx: %v", err)
	}
	registration, err := stores.Registrations.Get(ctx, registrationID)
	if err != nil {
		t.Fatalf("reload registration: %v", err)
	}
	if registration.Status != model.RegistrationStatusActive {
		t.Fatalf("registration status = %q, want active", registration.Status)
	}

	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Registrations.ActivateTx(ctx, tx, registrationID)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second ActivateTx err = %v, want ErrConflict", err)
	}
}

// TestPaymentCreateAndLookup 覆盖流水写入、按 transaction_id 幂等查询与唯一约束。
func TestPaymentCreateAndLookup(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()
	orderID, _, _ := mustPendingOrderFixture(t, stores, "2027010109300012345680")

	transactionID := "4200001234202701011234567890"
	notifiedAt := "2027-01-01T10:00:01+08:00"
	raw := `{"id":"ev-store-1"}`

	// 未写入 → (nil, nil)。
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		got, err := stores.Payments.GetByTransactionIDTx(ctx, tx, transactionID)
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatalf("missing payment = %+v, want nil", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("tx: %v", err)
	}

	var paymentID int64
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		paymentID, err = stores.Payments.CreateTx(ctx, tx, PaymentParams{
			OrderID: orderID, Type: model.PaymentTypePay, TransactionID: &transactionID,
			Amount: 98000, Status: model.PaymentStatusSuccess,
			RawCallback: &raw, NotifiedAt: &notifiedAt,
		})
		return err
	}); err != nil {
		t.Fatalf("CreateTx: %v", err)
	}
	if paymentID <= 0 {
		t.Fatalf("payment id = %d", paymentID)
	}

	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		got, err := stores.Payments.GetByTransactionIDTx(ctx, tx, transactionID)
		if err != nil {
			return err
		}
		if got == nil || got.ID != paymentID || got.OrderID != orderID ||
			got.Type != model.PaymentTypePay || got.Status != model.PaymentStatusSuccess ||
			got.Amount != 98000 || *got.TransactionID != transactionID ||
			*got.RawCallback != raw || *got.NotifiedAt != notifiedAt {
			t.Fatalf("payment = %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("tx: %v", err)
	}

	// 同一 transaction_id 重复写入 → 唯一冲突（服务层据此幂等）。
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := stores.Payments.CreateTx(ctx, tx, PaymentParams{
			OrderID: orderID, Type: model.PaymentTypePay, TransactionID: &transactionID,
			Amount: 98000, Status: model.PaymentStatusSuccess,
		})
		return err
	})
	if !IsUniqueViolation(err) {
		t.Fatalf("duplicate transaction_id err = %v, want unique violation", err)
	}
}
