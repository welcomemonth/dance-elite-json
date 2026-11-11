package service

import (
	"context"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// TestAdminOrderListFilters 覆盖订单列表的默认口径、筛选组合与分页。
func TestAdminOrderListFilters(t *testing.T) {
	f := newAdminQueryFixture(t)
	u11 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU11, model.DanceTypeClassical)
	u13 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU13, model.DanceTypeFolk)

	a := f.seed(t, "A", "ORD-A", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "张三")
	f.seed(t, "B", "ORD-B", model.OrderStatusCanceled,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "李四")
	f.seed(t, "C", "ORD-C", model.OrderStatusRefunding,
		model.OrderSourceMiniapp, model.DanceTypeFolk, u13, "王五")
	f.seed(t, "D", "ORD-D", model.OrderStatusRefunded,
		model.OrderSourceManual, model.DanceTypeFolk, u13, "赵六")
	f.seed(t, "E", "ORD-E", model.OrderStatusPaid,
		model.OrderSourceManual, model.DanceTypeClassical, u11, "钱七")
	f.seed(t, "F", "ORD-F", model.OrderStatusPendingPayment,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "孙八")

	f.setPaidAt(t, "ORD-A", "2027-01-01T10:00:00+08:00")
	f.setPaidAt(t, "ORD-C", "2027-02-10T10:00:00+08:00")
	f.setPaidAt(t, "ORD-D", "2027-03-10T10:00:00+08:00")
	f.setPaidAt(t, "ORD-E", "2027-04-10T10:00:00+08:00")
	f.addPayPayment(t, "ORD-A", "4200XYZ0001")

	ctx := context.Background()
	cases := []struct {
		name string
		q    model.AdminOrderListQuery
		want []string
	}{
		{"默认排除已取消与待支付", model.AdminOrderListQuery{}, []string{"ORD-E", "ORD-D", "ORD-C", "ORD-A"}},
		{"含已取消", model.AdminOrderListQuery{IncludeCanceled: true},
			[]string{"ORD-E", "ORD-D", "ORD-C", "ORD-B", "ORD-A"}},
		{"显式查已取消", model.AdminOrderListQuery{Status: model.OrderStatusCanceled}, []string{"ORD-B"}},
		{"来源后台人工", model.AdminOrderListQuery{Source: model.OrderSourceManual}, []string{"ORD-E", "ORD-D"}},
		{"组别", model.AdminOrderListQuery{Level: model.GroupLevelU13}, []string{"ORD-D", "ORD-C"}},
		{"舞种", model.AdminOrderListQuery{DanceType: model.DanceTypeFolk}, []string{"ORD-D", "ORD-C"}},
		{"赛事", model.AdminOrderListQuery{EventID: f.eventID}, []string{"ORD-E", "ORD-D", "ORD-C", "ORD-A"}},
		{"姓名关键字", model.AdminOrderListQuery{Keyword: "王五"}, []string{"ORD-C"}},
		{"订单号关键字", model.AdminOrderListQuery{Keyword: "ORD-D"}, []string{"ORD-D"}},
		{"微信支付单号关键字", model.AdminOrderListQuery{Keyword: "XYZ"}, []string{"ORD-A"}},
		{"支付时间范围（日期形式）", model.AdminOrderListQuery{PaidFrom: "2027-02-01", PaidTo: "2027-02-28"},
			[]string{"ORD-C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := f.orderSvc.List(ctx, tc.q)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			got := orderNosOf(resp.List)
			if len(got) != len(tc.want) {
				t.Fatalf("orders = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("orders = %v, want %v", got, tc.want)
				}
			}
		})
	}

	// 分页：total 为筛选总数，list 按页返回。
	page, err := f.orderSvc.List(ctx, model.AdminOrderListQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("page list: %v", err)
	}
	if page.Total != 4 || len(page.List) != 2 || page.Page != 2 || page.PageSize != 2 {
		t.Fatalf("page result = total %d, len %d, page %d/%d", page.Total, len(page.List), page.Page, page.PageSize)
	}

	// 响应不含任何证件号字段，选手以 player_no + id_type 标识。
	if resp, err := f.orderSvc.List(ctx, model.AdminOrderListQuery{Keyword: "ORD-A"}); err != nil {
		t.Fatalf("list ORD-A: %v", err)
	} else if len(resp.List) != 1 {
		t.Fatalf("ORD-A rows = %d, want 1", len(resp.List))
	} else {
		item := resp.List[0]
		if item.PlayerNo != a.PlayerNo || item.IDType != "id_card" || item.TransactionID == nil ||
			*item.TransactionID != "4200XYZ0001" {
			t.Fatalf("item = %+v", item)
		}
	}

	// 非法筛选值 → 40001。
	invalid := []model.AdminOrderListQuery{
		{Status: "bogus"},
		{Level: "U99"},
		{DanceType: "ballet"},
		{Source: "wechat"},
		{PaidFrom: "not-a-date"},
		{PaidFrom: "2027-03-01", PaidTo: "2027-02-01"},
	}
	for _, q := range invalid {
		if _, err := f.orderSvc.List(ctx, q); mustBizCode(t, err) != 40001 {
			t.Fatalf("query %+v should be 40001, got %v", q, err)
		}
	}
}

// orderNosOf 提取订单号列表。
func orderNosOf(items []model.AdminOrderItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.OrderNo)
	}
	return out
}
