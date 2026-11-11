package service

import (
	"context"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// TestAdminRegistrationListFilters 覆盖报名名单的状态/资料完成度筛选、缺失项标注与分页。
func TestAdminRegistrationListFilters(t *testing.T) {
	f := newAdminQueryFixture(t)
	u11 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU11, model.DanceTypeClassical)
	u13 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU13, model.DanceTypeFolk)

	full := f.seed(t, "R1", "ORD-R1", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "完整选手")
	noAudio := f.seed(t, "R2", "ORD-R2", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "缺音频选手")
	noPhoto := f.seed(t, "R3", "ORD-R3", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "缺照片选手")
	canceled := f.seed(t, "R4", "ORD-R4", model.OrderStatusCanceled,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "已取消选手")
	folk := f.seed(t, "R5", "ORD-R5", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeFolk, u13, "民族选手")

	f.fillMaterials(t, full.RegistrationID, allMaterialFields...)
	f.fillMaterials(t, noAudio.RegistrationID,
		withoutField(withoutField(allMaterialFields, model.MaterialFieldAudio),
			model.MaterialFieldContactPhone)...)
	f.fillMaterials(t, noPhoto.RegistrationID, withoutField(allMaterialFields, model.MaterialFieldPhoto)...)
	f.fillMaterials(t, canceled.RegistrationID, allMaterialFields...)
	f.fillMaterials(t, folk.RegistrationID, allMaterialFields...)
	f.setUserPhone(t, noAudio.UserID, "13800000002")

	ctx := context.Background()
	cases := []struct {
		name string
		q    model.AdminRegistrationListQuery
		want []int64
	}{
		{"默认仅 active", model.AdminRegistrationListQuery{},
			[]int64{full.RegistrationID, noAudio.RegistrationID, noPhoto.RegistrationID, folk.RegistrationID}},
		{"资料已齐", model.AdminRegistrationListQuery{MaterialState: "complete"},
			[]int64{full.RegistrationID, folk.RegistrationID}},
		{"资料未齐", model.AdminRegistrationListQuery{MaterialState: "incomplete"},
			[]int64{noAudio.RegistrationID, noPhoto.RegistrationID}},
		{"缺音频", model.AdminRegistrationListQuery{MissingItem: model.MaterialFieldAudio},
			[]int64{noAudio.RegistrationID}},
		{"缺照片", model.AdminRegistrationListQuery{MissingItem: model.MaterialFieldPhoto},
			[]int64{noPhoto.RegistrationID}},
		{"显式查已取消", model.AdminRegistrationListQuery{Status: model.RegistrationStatusCanceled},
			[]int64{canceled.RegistrationID}},
		{"已取消且资料已齐", model.AdminRegistrationListQuery{
			Status: model.RegistrationStatusCanceled, MaterialState: "complete"},
			[]int64{canceled.RegistrationID}},
		{"组别", model.AdminRegistrationListQuery{GroupID: u13}, []int64{folk.RegistrationID}},
		{"级别", model.AdminRegistrationListQuery{Level: model.GroupLevelU13}, []int64{folk.RegistrationID}},
		{"舞种", model.AdminRegistrationListQuery{DanceType: model.DanceTypeFolk}, []int64{folk.RegistrationID}},
		{"姓名关键字", model.AdminRegistrationListQuery{Keyword: "缺照片"}, []int64{noPhoto.RegistrationID}},
		{"选手编号关键字", model.AdminRegistrationListQuery{Keyword: noAudio.PlayerNo},
			[]int64{noAudio.RegistrationID}},
		{"订单号关键字", model.AdminRegistrationListQuery{Keyword: "ORD-R1"}, []int64{full.RegistrationID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := f.regSvc.List(ctx, tc.q)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			got := registrationIDsOf(resp.List)
			assertIDsEqual(t, got, tc.want)
		})
	}

	// 缺失项与联系电话回退：R2 缺 audio，联系电话取用户手机号。
	resp, err := f.regSvc.List(ctx, model.AdminRegistrationListQuery{Keyword: noAudio.PlayerNo})
	if err != nil {
		t.Fatalf("list R2: %v", err)
	}
	if len(resp.List) != 1 {
		t.Fatalf("R2 rows = %d", len(resp.List))
	}
	item := resp.List[0]
	if item.MaterialTotal != 8 || item.MaterialCompleted != 6 ||
		len(item.MissingItems) != 2 || item.MissingItems[0] != model.MaterialFieldAudio ||
		item.MissingItems[1] != model.MaterialFieldContactPhone {
		t.Fatalf("material status = %d/%d %v", item.MaterialCompleted, item.MaterialTotal, item.MissingItems)
	}
	if item.ContactPhone == nil || *item.ContactPhone != "13800000002" {
		t.Fatalf("contact_phone = %v, want user phone fallback", item.ContactPhone)
	}

	// 分页。
	page, err := f.regSvc.List(ctx, model.AdminRegistrationListQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("page list: %v", err)
	}
	if page.Total != 4 || len(page.List) != 2 {
		t.Fatalf("page result = total %d, len %d", page.Total, len(page.List))
	}

	// 非法筛选值 → 40001。
	invalid := []model.AdminRegistrationListQuery{
		{Status: "bogus"},
		{Level: "U99"},
		{DanceType: "ballet"},
		{MaterialState: "partial"},
		{MissingItem: "teacher_name"},
	}
	for _, q := range invalid {
		if _, err := f.regSvc.List(ctx, q); mustBizCode(t, err) != 40001 {
			t.Fatalf("query %+v should be 40001, got %v", q, err)
		}
	}
}

// withoutField 返回去掉指定字段后的副本。
func withoutField(fields []string, drop string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != drop {
			out = append(out, field)
		}
	}
	return out
}

// registrationIDsOf 提取报名单 ID 列表。
func registrationIDsOf(items []model.AdminRegistrationItem) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.RegistrationID)
	}
	return out
}

// assertIDsEqual 断言两个 ID 集合完全一致（忽略顺序）。
func assertIDsEqual(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	seen := make(map[int64]int, len(want))
	for _, id := range want {
		seen[id]++
	}
	for _, id := range got {
		seen[id]--
		if seen[id] < 0 {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}
