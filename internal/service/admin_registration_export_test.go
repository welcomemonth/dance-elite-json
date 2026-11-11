package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/xuri/excelize/v2"
)

// TestAdminRegistrationExport 覆盖按赛事导出名单的筛选、列内容与审计留痕。
func TestAdminRegistrationExport(t *testing.T) {
	f := newAdminQueryFixture(t)
	ctx := context.Background()
	u11 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU11, model.DanceTypeClassical)
	adminID := mustAdminID(t, f.regFixture, "admin")

	complete := f.seed(t, "E1", "ORD-EXP-1", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "完整选手")
	incomplete := f.seed(t, "E2", "ORD-EXP-2", model.OrderStatusPaid,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "缺音频选手")
	canceled := f.seed(t, "E3", "ORD-EXP-3", model.OrderStatusCanceled,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "已取消选手")
	f.fillMaterials(t, complete.RegistrationID, allMaterialFields...)
	f.fillMaterials(t, incomplete.RegistrationID,
		withoutField(allMaterialFields, model.MaterialFieldAudio)...)
	f.fillMaterials(t, canceled.RegistrationID, allMaterialFields...)
	mustExecSQL(t, f.db,
		`UPDATE registrations SET contact_phone = '13800000002' WHERE id = ?`, incomplete.RegistrationID)

	file, err := f.regSvc.Export(ctx, adminID, f.eventID, false)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if file.Rows != 2 || !strings.HasSuffix(file.Name, ".xlsx") {
		t.Fatalf("file = %+v, want 2 rows xlsx", file)
	}
	if n := countAdminLogOf(t, f.db, "admin.registration.export", "event", f.eventID); n != 1 {
		t.Fatalf("export admin logs = %d, want 1", n)
	}

	sheet := readExportSheet(t, file.Content)
	if len(sheet) != 3 { // 表头 + 2 行有效报名
		t.Fatalf("sheet rows = %d, want 3: %v", len(sheet), sheet)
	}
	assertExportHeader(t, sheet[0])
	// 行内包含选手编号、联系电话、资料完成度与缺失项中文。
	body := strings.Join(flattenRows(sheet[1:]), "|")
	for _, want := range []string{complete.PlayerNo, incomplete.PlayerNo, "13800000002", "7/8", "舞蹈音频"} {
		if !strings.Contains(body, want) {
			t.Fatalf("export body missing %q: %s", want, body)
		}
	}
	// 不含证件号任何形态：哈希、证件号字段名、号码痕迹。
	assertNoIDNumber(t, body)

	// include_canceled=true 含已取消留档，并标注状态。
	file, err = f.regSvc.Export(ctx, adminID, f.eventID, true)
	if err != nil {
		t.Fatalf("export with canceled: %v", err)
	}
	sheet = readExportSheet(t, file.Content)
	if len(sheet) != 4 {
		t.Fatalf("sheet rows with canceled = %d, want 4", len(sheet))
	}
	body = strings.Join(flattenRows(sheet[1:]), "|")
	if !strings.Contains(body, canceled.PlayerNo) || !strings.Contains(body, "已取消") {
		t.Fatalf("export should include canceled row: %s", body)
	}

	// 赛事不存在 → 40401。
	if _, err := f.regSvc.Export(ctx, adminID, 999999, false); mustBizCode(t, err) != 40401 {
		t.Fatalf("missing event should be 40401, got %v", err)
	}
}

// readExportSheet 打开导出工作簿并返回全部行。
func readExportSheet(t *testing.T, content []byte) [][]string {
	t.Helper()
	xl, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("open export workbook: %v", err)
	}
	defer func() { _ = xl.Close() }()
	rows, err := xl.GetRows(exportSheetName)
	if err != nil {
		t.Fatalf("read export sheet: %v", err)
	}
	return rows
}

// assertExportHeader 断言导出表头与约定列一致。
func assertExportHeader(t *testing.T, header []string) {
	t.Helper()
	want := []string{"订单号", "选手姓名", "选手编号", "组别", "舞种", "机构",
		"资料完成度", "缺失资料项", "联系电话", "报名状态", "报名时间"}
	if len(header) != len(want) {
		t.Fatalf("header = %v, want %v", header, want)
	}
	for i := range want {
		if header[i] != want[i] {
			t.Fatalf("header[%d] = %q, want %q", i, header[i], want[i])
		}
	}
}

// flattenRows 把二维单元格拼接为一维字符串。
func flattenRows(rows [][]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

// assertNoIDNumber 断言导出内容不含证件号字段或哈希痕迹。
func assertNoIDNumber(t *testing.T, content string) {
	t.Helper()
	for _, forbidden := range []string{"id_card", "证件号", "hash-"} {
		if strings.Contains(strings.ToLower(content), forbidden) {
			t.Fatalf("export must not contain %q: %s", forbidden, content)
		}
	}
}
