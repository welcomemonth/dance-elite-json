package service

import (
	"errors"
	"testing"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// testStringPtr 返回字符串指针（测试夹具用，避免引入生产符号）。
func testStringPtr(s string) *string { return &s }

// refundRuleTime 是退款规则测试基准时刻：报名截止前 1 小时。
var refundRuleTime = time.Date(2027, 1, 1, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))

// mustBizCode 断言错误是业务错误并返回错误码。
func mustBizCode(t *testing.T, err error) int {
	t.Helper()
	var biz *httpx.BizError
	if !errors.As(err, &biz) {
		t.Fatalf("expected BizError, got %v", err)
	}
	return biz.Code
}

// TestCheckRefundEligibility 覆盖订单状态与报名截止两类退款规则。
func TestCheckRefundEligibility(t *testing.T) {
	event := &model.Event{
		ID:       1,
		RegEndAt: testStringPtr("2027-01-01T11:00:00+08:00"),
	}

	cases := []struct {
		name     string
		status   string
		now      time.Time
		wantCode int // 0 表示通过
	}{
		{name: "已支付且截止前", status: model.OrderStatusPaid, now: refundRuleTime},
		{name: "已支付且截止前一分钟", status: model.OrderStatusPaid, now: refundRuleTime.Add(59 * time.Minute)},
		{name: "已支付且恰好截止", status: model.OrderStatusPaid, now: refundRuleTime.Add(time.Hour), wantCode: httpx.CodeRefundNotAllowed},
		{name: "已支付且过截止", status: model.OrderStatusPaid, now: refundRuleTime.Add(2 * time.Hour), wantCode: httpx.CodeRefundNotAllowed},
		{name: "待支付", status: model.OrderStatusPendingPayment, now: refundRuleTime, wantCode: httpx.CodeRefundNotAllowed},
		{name: "已取消", status: model.OrderStatusCanceled, now: refundRuleTime, wantCode: httpx.CodeRefundNotAllowed},
		{name: "退款中", status: model.OrderStatusRefunding, now: refundRuleTime, wantCode: httpx.CodeRefundNotAllowed},
		{name: "已退款", status: model.OrderStatusRefunded, now: refundRuleTime, wantCode: httpx.CodeRefundNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRefundEligibility(&model.Order{ID: 1, Status: tc.status}, event, tc.now)
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if got := mustBizCode(t, err); got != tc.wantCode {
				t.Fatalf("code = %d, want %d (err=%v)", got, tc.wantCode, err)
			}
		})
	}
}

// TestCheckRefundEligibilityTimelineMissing 覆盖时间线缺失/格式异常：属数据问题，
// 返回非业务错误（路由映射 50001），而不是 41015 误导管理员。
func TestCheckRefundEligibilityTimelineMissing(t *testing.T) {
	order := &model.Order{ID: 1, Status: model.OrderStatusPaid}

	for _, event := range []*model.Event{
		{ID: 2},
		{ID: 3, RegEndAt: testStringPtr("")},
		{ID: 4, RegEndAt: testStringPtr("not-a-time")},
	} {
		err := checkRefundEligibility(order, event, refundRuleTime)
		if err == nil {
			t.Fatalf("event %d: expected error", event.ID)
		}
		var biz *httpx.BizError
		if errors.As(err, &biz) {
			t.Fatalf("event %d: expected plain error, got BizError %d", event.ID, biz.Code)
		}
	}
}
