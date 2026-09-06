package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/xuri/excelize/v2"
)

// 序号 | 姓名 | 身份证 | 积分赛 | 组别 | 剧目 | 指导老师 | 所属机构 | 成绩 | 本站积分
type PlayerScoreRecord struct {
	SerialNo     int    `json:"serial_no"`     // 序号
	Name         string `json:"name"`          // 姓名
	IDCard       string `json:"id_card"`       // 身份证
	AgeGroup     string `json:"age_group"`     // 组别
	ShowName     string `json:"show_name"`     // 剧目
	Tutor        string `json:"tutor"`         // 指导老师
	Organization string `json:"organization"`  // 所属机构
	Award        string `json:"award"`         // 成绩
	CurrentPoint int    `json:"current_point"` // 本站积分
}

// 字段别名配置（key 必须和结构体字段名对应，方便后续扩展）
var fieldAliases = map[string][]string{
	"SerialNo":     {"序号", "编号", "No", "Serial"},
	"Name":         {"姓名", "选手姓名", "运动员", "Name", "选手"},
	"IDCard":       {"身份证", "身份证号", "证件号", "ID Card", "IdCard", "身份证号码"},
	"AgeGroup":     {"组别", "年龄组", "Age Group", "分组"},
	"ShowName":     {"剧目", "节目", "作品名称", "Show", "曲目"},
	"Tutor":        {"指导老师", "老师", "Tutor", "指导教师"},
	"Organization": {"所属机构", "单位", "学校", "机构", "Organization", "代表队"},
	"Award":        {"成绩", "得分", "分数", "Score", "Award", "最终成绩", "获奖名称"},
	"CurrentPoint": {"本站积分", "积分", "Point", "本站得分", "Current Point"},
}

// ParseExcelToStruct 对外入口（保持原有签名）
func ParseExcelToStruct(filePath string) ([]PlayerScoreRecord, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()
	return parseExcelFile(f)
}

// parseExcelFile 真正的解析逻辑（可被测试直接调用）
func parseExcelFile(f *excelize.File) ([]PlayerScoreRecord, error) {
	sheetList := f.GetSheetList()
	if len(sheetList) == 0 {
		return nil, fmt.Errorf("文件中没有任何工作表")
	}

	var (
		allRecords []PlayerScoreRecord
		baseColMap map[string]int // 第一个 Sheet 的映射，后续无表头时复用
	)

	for sheetIdx, sheetName := range sheetList {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("警告: 读取 Sheet [%s] 失败: %v，请手动检查\n", sheetName, err)
		}
		if len(rows) == 0 {
			continue
		}

		var colMap map[string]int
		var dataStart int // 数据从第几行开始（0-based）

		if sheetIdx == 0 {
			// ========== 第一个 Sheet：固定有表头 ==========
			if len(rows) < 2 {
				logger.Infof("警告: 第一个 Sheet [%s] 数据不足，已跳过\n", sheetName)
				continue
			}
			baseColMap = buildColumnIndexMap(rows[0])
			colMap = baseColMap
			dataStart = 1

			// 可选：检查关键字段
			if _, ok := colMap["Name"]; !ok {
				return nil, fmt.Errorf("第一个 Sheet [%s] 未匹配到「姓名」列，请检查表头", sheetName)
			}
		} else {
			// ========== 后续 Sheet：判断是否有自己的表头 ==========
			if isHeaderRow(rows[0]) {
				// 有自己的表头
				colMap = buildColumnIndexMap(rows[0])
				dataStart = 1
				fmt.Printf("Sheet [%s] 检测到独立表头，使用自己的映射\n", sheetName)
			} else {
				// 没有表头，复用第一个 Sheet 的映射
				if baseColMap == nil {
					fmt.Printf("警告: Sheet [%s] 无表头，且第一个 Sheet 映射无效，已跳过\n", sheetName)
					continue
				}
				colMap = baseColMap
				dataStart = 0 // 从第一行开始全部当数据
				fmt.Printf("Sheet [%s] 未检测到表头，复用第一个 Sheet 的映射\n", sheetName)
			}
		}

		// 解析当前 Sheet 的数据行
		for i := dataStart; i < len(rows); i++ {
			record := fillRecord(rows[i], colMap)

			// 跳过空行
			if record.Name == "" && record.IDCard == "" {
				continue
			}
			allRecords = append(allRecords, record)
		}
	}

	if len(allRecords) == 0 {
		return nil, fmt.Errorf("未解析到任何有效数据")
	}
	return allRecords, nil
}

// 判断一行是否像表头（能否匹配到关键字段）
func isHeaderRow(row []string) bool {
	if len(row) == 0 {
		return false
	}

	// 只要能匹配到「姓名」或「身份证」其中一个，就认为是表头
	tempMap := buildColumnIndexMap(row)
	_, hasName := tempMap["Name"]
	_, hasIDCard := tempMap["IDCard"]
	return hasName || hasIDCard
}

// 根据表头构建 字段名 → 列索引
func buildColumnIndexMap(header []string) map[string]int {
	colMap := make(map[string]int)

	normalized := make([]string, len(header))
	for i, h := range header {
		normalized[i] = strings.TrimSpace(strings.ToLower(h))
	}

	for field, aliases := range fieldAliases {
		for _, alias := range aliases {
			aliasLower := strings.ToLower(strings.TrimSpace(alias))
			for colIdx, h := range normalized {
				if h == aliasLower || strings.Contains(h, aliasLower) {
					colMap[field] = colIdx
					goto nextField
				}
			}
		}
	nextField:
	}
	return colMap
}

// 按映射填充结构体
func fillRecord(row []string, colMap map[string]int) PlayerScoreRecord {
	var r PlayerScoreRecord

	if idx, ok := colMap["SerialNo"]; ok && idx < len(row) {
		r.SerialNo = toInt(row[idx])
	}
	if idx, ok := colMap["Name"]; ok && idx < len(row) {
		r.Name = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["IDCard"]; ok && idx < len(row) {
		r.IDCard = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["AgeGroup"]; ok && idx < len(row) {
		r.AgeGroup = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["ShowName"]; ok && idx < len(row) {
		r.ShowName = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["Tutor"]; ok && idx < len(row) {
		r.Tutor = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["Organization"]; ok && idx < len(row) {
		r.Organization = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["Award"]; ok && idx < len(row) {
		r.Award = strings.TrimSpace(row[idx])
	}
	if idx, ok := colMap["CurrentPoint"]; ok && idx < len(row) {
		r.CurrentPoint = toInt(row[idx])
	}
	return r
}

func toInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}

// func toFloat(s string) float64 {
// 	s = strings.TrimSpace(s)
// 	if s == "" {
// 		return 0
// 	}
// 	s = strings.ReplaceAll(s, ",", "")
// 	s = strings.ReplaceAll(s, "%", "")
// 	v, _ := strconv.ParseFloat(s, 64)
// 	return v
// }
