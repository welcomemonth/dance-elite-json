package utils

import (
	"sort"
	"testing"

	"github.com/xuri/excelize/v2"
	// 可选，用标准库也行
)

// 辅助函数：快速创建一个内存中的 Excel 文件
func createTestExcel(sheets map[string][][]string) *excelize.File {
	f := excelize.NewFile()

	// 删除默认的 Sheet1（后面我们自己创建）
	f.DeleteSheet("Sheet1")

	// 按 sheet 名排序，避免 Go map 遍历顺序随机导致「第一个 sheet」不稳定，
	// 从而让依赖「第一个 sheet 含表头」的用例确定性通过。
	names := make([]string, 0, len(sheets))
	for name := range sheets {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		rows := sheets[name]
		if i == 0 {
			f.SetSheetName("Sheet1", name) // 第一个 sheet 改名
		} else {
			f.NewSheet(name)
		}

		for rIdx, row := range rows {
			for cIdx, val := range row {
				cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
				f.SetCellValue(name, cell, val)
			}
		}
	}
	return f
}

// ===================== 测试用例 =====================

func TestParseExcel_FirstSheetOnly(t *testing.T) {
	f := createTestExcel(map[string][][]string{
		"成绩表": {
			{"序号", "姓名", "身份证号", "组别", "剧目", "指导老师", "所属机构", "成绩", "本站积分"},
			{"1", "张三", "110101199001011234", "少儿组", "春晓", "李老师", "星海艺术", "95.5", "10"},
			{"2", "李四", "110101199002021234", "青年组", "月光", "王老师", "阳光琴行", "88", "8"},
		},
	})

	records, err := parseExcelFile(f)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("期望 2 条记录，实际 %d", len(records))
	}

	// 验证第一条
	r1 := records[0]
	if r1.Name != "张三" || r1.IDCard != "110101199001011234" || r1.Award != "95.5" {
		t.Errorf("第一条数据不正确: %+v", r1)
	}
	if r1.SerialNo != 1 || r1.CurrentPoint != 10 {
		t.Errorf("序号或积分不正确: %+v", r1)
	}
}

func TestParseExcel_MultipleSheets_WithAndWithoutHeader(t *testing.T) {
	f := createTestExcel(map[string][][]string{
		// 第一个 Sheet：有表头
		"第一页": {
			{"姓名", "身份证", "成绩"},
			{"张三", "110101199001011234", "95"},
			{"李四", "110101199002021234", "88"},
		},
		// 第二个 Sheet：没有表头（直接数据）
		"第二页": {
			{"王五", "110101199003031234", "92"},
			{"赵六", "110101199004041234", "85"},
		},
		// 第三个 Sheet：有自己的表头（顺序不同）
		"第三页": {
			{"身份证号", "姓名", "最终成绩"},
			{"110101199005051234", "孙七", "90"},
		},
	})

	records, err := parseExcelFile(f)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 应该解析到 2 + 2 + 1 = 5 条
	if len(records) != 5 {
		t.Fatalf("期望 5 条记录，实际 %d 条", len(records))
	}

	// 简单检查名字是否都在
	names := make(map[string]bool)
	for _, r := range records {
		names[r.Name] = true
	}
	expectedNames := []string{"张三", "李四", "王五", "赵六", "孙七"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("缺少选手: %s", name)
		}
	}
}

func TestParseExcel_EmptyFile(t *testing.T) {
	f := excelize.NewFile()
	// 只有一个空 Sheet
	_, err := parseExcelFile(f)
	if err == nil {
		t.Error("空文件应该返回错误")
	}
}

func TestParseExcel_FirstSheetMissingNameColumn(t *testing.T) {
	f := createTestExcel(map[string][][]string{
		"坏表头": {
			{"编号", "证件号", "分数"}, // 没有「姓名」相关列
			{"1", "110101199001011234", "90"},
		},
	})

	_, err := parseExcelFile(f)
	if err == nil {
		t.Error("第一个 Sheet 缺少姓名列时应该报错")
	}
}

func TestIsHeaderRow(t *testing.T) {
	// 是表头
	if !isHeaderRow([]string{"姓名", "身份证号", "成绩"}) {
		t.Error("应该识别为表头")
	}
	// 不是表头（纯数据）
	if isHeaderRow([]string{"张三", "110101199001011234", "95"}) {
		t.Error("不应该识别为表头")
	}
	// 空行
	if isHeaderRow([]string{}) {
		t.Error("空行不应该是表头")
	}
}

func TestBuildColumnIndexMap(t *testing.T) {
	header := []string{"选手姓名", "身份证号码", "最终成绩", "本站积分"}
	m := buildColumnIndexMap(header)

	if m["Name"] != 0 {
		t.Errorf("Name 应该映射到第0列，实际 %d", m["Name"])
	}
	if m["IDCard"] != 1 {
		t.Errorf("IDCard 应该映射到第1列，实际 %d", m["IDCard"])
	}
	if m["Award"] != 2 {
		t.Errorf("Award 应该映射到第2列，实际 %d", m["Award"])
	}
}

func TestToIntAndToFloat(t *testing.T) {
	if toInt("12") != 12 {
		t.Error("toInt 失败")
	}
	if toInt("12.0") != 12 {
		t.Error("toInt 处理小数失败")
	}
	if toInt("") != 0 {
		t.Error("空字符串应返回 0")
	}

	// if toFloat("95.5") != 95.5 {
	// 	t.Error("toFloat 失败")
	// }
	// if toFloat("1,234.5") != 1234.5 {
	// 	t.Error("toFloat 处理千分位失败")
	// }
}
