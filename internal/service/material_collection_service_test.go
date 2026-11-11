package service

import (
	"context"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// TestMaterialCollectionList 覆盖催收名单的完成度口径、筛选与截止状态。
func TestMaterialCollectionList(t *testing.T) {
	f := newAdminQueryFixture(t)
	ctx := context.Background()
	u11 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU11, model.DanceTypeClassical)
	u13 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU13, model.DanceTypeFolk)

	noAudio := f.seed(t, "M1", "ORD-M1", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "缺音频")
	noPhoto := f.seed(t, "M2", "ORD-M2", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeFolk, u13, "缺照片")
	complete := f.seed(t, "M3", "ORD-M3", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "资料齐全")
	canceled := f.seed(t, "M4", "ORD-M4", model.OrderStatusCanceled,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "已取消")

	f.fillMaterials(t, noAudio.RegistrationID,
		withoutField(allMaterialFields, model.MaterialFieldAudio)...)
	f.fillMaterials(t, noPhoto.RegistrationID,
		withoutField(allMaterialFields, model.MaterialFieldPhoto)...)
	f.fillMaterials(t, complete.RegistrationID, allMaterialFields...)
	f.fillMaterials(t, canceled.RegistrationID, allMaterialFields...)

	cases := []struct {
		name string
		q    model.AdminMissingMaterialQuery
		want []int64
	}{
		{"默认：active 且核心三项缺失", model.AdminMissingMaterialQuery{},
			[]int64{noAudio.RegistrationID, noPhoto.RegistrationID}},
		{"缺音频", model.AdminMissingMaterialQuery{MissingItem: model.MaterialFieldAudio},
			[]int64{noAudio.RegistrationID}},
		{"缺照片", model.AdminMissingMaterialQuery{MissingItem: model.MaterialFieldPhoto},
			[]int64{noPhoto.RegistrationID}},
		{"组别", model.AdminMissingMaterialQuery{Level: model.GroupLevelU13},
			[]int64{noPhoto.RegistrationID}},
		{"舞种", model.AdminMissingMaterialQuery{DanceType: model.DanceTypeClassical},
			[]int64{noAudio.RegistrationID}},
		{"未截止", model.AdminMissingMaterialQuery{DeadlineState: "open"},
			[]int64{noAudio.RegistrationID, noPhoto.RegistrationID}},
		{"已截止", model.AdminMissingMaterialQuery{DeadlineState: "closed"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := f.collection.List(ctx, tc.q)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			assertIDsEqual(t, registrationIDsFromMissing(resp.List), tc.want)
		})
	}

	// 行内容：缺失项、资料截止、未联系。
	resp, err := f.collection.List(ctx, model.AdminMissingMaterialQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, item := range resp.List {
		if item.MaterialTotal != 8 || item.MaterialCompleted != 7 || len(item.MissingItems) != 1 {
			t.Fatalf("item material = %+v", item)
		}
		if item.MaterialDeadline == nil || item.Contacted {
			t.Fatalf("item deadline/contacted = %+v", item)
		}
	}

	// 时间线移到过去后全部归入已截止。
	f.setTimelinePast(t)
	resp, err = f.collection.List(ctx, model.AdminMissingMaterialQuery{DeadlineState: "closed"})
	if err != nil {
		t.Fatalf("closed list: %v", err)
	}
	if len(resp.List) != 2 {
		t.Fatalf("closed rows = %d, want 2", len(resp.List))
	}

	// 非法筛选值 → 40001。
	invalid := []model.AdminMissingMaterialQuery{
		{MissingItem: "teacher_name"},
		{DeadlineState: "locked"},
		{Level: "U99"},
		{DanceType: "ballet"},
	}
	for _, q := range invalid {
		if _, err := f.collection.List(ctx, q); mustBizCode(t, err) != 40001 {
			t.Fatalf("query %+v should be 40001, got %v", q, err)
		}
	}
}

// TestMaterialCollectionMarkContacted 覆盖标记已联系的留痕、重复调用与截止锁定。
func TestMaterialCollectionMarkContacted(t *testing.T) {
	f := newAdminQueryFixture(t)
	ctx := context.Background()
	u11 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU11, model.DanceTypeClassical)
	adminID := mustAdminID(t, f.regFixture, "admin")
	seed := f.seed(t, "C1", "ORD-C1", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "催收选手")
	f.fillMaterials(t, seed.RegistrationID,
		withoutField(allMaterialFields, model.MaterialFieldAudio)...)

	resp, err := f.collection.MarkContacted(ctx, adminID, seed.RegistrationID, " 已电话提醒 ")
	if err != nil {
		t.Fatalf("mark contacted: %v", err)
	}
	if resp.RegistrationID != seed.RegistrationID || !resp.Contacted {
		t.Fatalf("resp = %+v", resp)
	}
	var note string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT note FROM material_contacts WHERE registration_id = ?`,
		seed.RegistrationID).Scan(&note); err != nil {
		t.Fatalf("query contact: %v", err)
	}
	if note != "已电话提醒" {
		t.Fatalf("note = %q, want trimmed", note)
	}
	if n := countAdminLogOf(t, f.db, "admin.material.contacted", "registration", seed.RegistrationID); n != 1 {
		t.Fatalf("contacted admin logs = %d, want 1", n)
	}

	// 名单回显已联系与时间。
	list, err := f.collection.List(ctx, model.AdminMissingMaterialQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.List) != 1 || !list.List[0].Contacted || list.List[0].ContactedAt == nil {
		t.Fatalf("list after contacted = %+v", list.List)
	}

	// 重复标记：追加历史记录，仍为已联系。
	if _, err := f.collection.MarkContacted(ctx, adminID, seed.RegistrationID, "第二次催收"); err != nil {
		t.Fatalf("second contacted: %v", err)
	}
	var contacts int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM material_contacts WHERE registration_id = ?`,
		seed.RegistrationID).Scan(&contacts); err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if contacts != 2 {
		t.Fatalf("contacts = %d, want 2 (history keeps both)", contacts)
	}

	// 备注超长 → 40001；报名单不存在 → 40401；非 active → 41009。
	if _, err := f.collection.MarkContacted(ctx, adminID, seed.RegistrationID,
		strings.Repeat("催", 201)); mustBizCode(t, err) != 40001 {
		t.Fatalf("long note should be 40001, got %v", err)
	}
	if _, err := f.collection.MarkContacted(ctx, adminID, 999999, "备注"); mustBizCode(t, err) != 40401 {
		t.Fatalf("missing registration should be 40401, got %v", err)
	}
	mustExecSQL(t, f.db, `UPDATE registrations SET status = 'withdrawn' WHERE id = ?`, seed.RegistrationID)
	if _, err := f.collection.MarkContacted(ctx, adminID, seed.RegistrationID, "备注"); mustBizCode(t, err) != 41009 {
		t.Fatalf("non-active should be 41009, got %v", err)
	}

	// 截止后锁定 → 41008。
	mustExecSQL(t, f.db, `UPDATE registrations SET status = 'active' WHERE id = ?`, seed.RegistrationID)
	f.setTimelinePast(t)
	if _, err := f.collection.MarkContacted(ctx, adminID, seed.RegistrationID, "备注"); mustBizCode(t, err) != 41008 {
		t.Fatalf("past deadline should be 41008, got %v", err)
	}
}

// registrationIDsFromMissing 提取催收名单的报名单 ID。
func registrationIDsFromMissing(items []model.AdminMissingMaterialItem) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.RegistrationID)
	}
	return out
}
