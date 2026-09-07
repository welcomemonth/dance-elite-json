package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/utils"
	"github.com/xuri/excelize/v2"
)

// ======================== 配置 ========================

const (
	defaultOutputDir  = "./data"
	defaultOutputFile = "ranking.jsonc"
)

// ======================== 入口 ========================

func main() {
	// 示例1：只解析一个新文件（覆盖写入）
	// err := ProcessExcelFiles([]string{"./data/excel/7月26.xlsx"}, "", false)

	// 示例2：在已有 ranking.jsonc 基础上合并新 Excel
	err := ProcessExcelFiles(
		[]string{"./data/excel/7月26.xlsx"}, // 要合并的新文件列表
		"./data/ranking.jsonc",             // 已有的排名文件（不存在则自动创建）
		true,                               // true = 合并模式
	)
	if err != nil {
		log.Fatal(err)
	}
}

// ProcessExcelFiles 统一入口
// excelFiles: 需要解析的 Excel 文件路径列表
// existingJSON: 已有的 ranking.jsonc 路径（合并模式时使用，空字符串表示不加载）
// mergeMode: true=合并到已有数据，false=只使用本次 Excel 数据
func ProcessExcelFiles(excelFiles []string, existingJSON string, mergeMode bool) error {
	// 1. 加载已有数据（如果开启合并）
	rankMap := make(map[string]map[string]*model.Dancer) // category -> idCard -> Dancer

	if mergeMode && existingJSON != "" {
		if err := loadExistingRanking(existingJSON, rankMap); err != nil {
			// 文件不存在时忽略，视为首次生成
			if !os.IsNotExist(err) {
				return fmt.Errorf("加载已有排名失败: %w", err)
			}
			log.Printf("提示: 已有排名文件不存在，将创建新文件")
		}
	}

	// 2. 解析并合并所有 Excel
	for _, filePath := range excelFiles {
		if err := parseExcelToRankMap(filePath, rankMap); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", filePath, err)
		}
	}

	// 3. 排序 + 转换为最终结构
	finalResult := convertAndSort(rankMap)

	// 4. 保存
	outputPath := filepath.Join(defaultOutputDir, defaultOutputFile)
	if err := saveRanking(finalResult, outputPath); err != nil {
		return err
	}

	fmt.Printf("✅ 处理完成，共 %d 个组别，已保存到: %s\n", len(finalResult), outputPath)
	return nil
}

// ======================== 核心解析逻辑 ========================

// parseExcelToRankMap 解析单个 Excel，并合并进 rankMap
func parseExcelToRankMap(filePath string, rankMap map[string]map[string]*model.Dancer) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	// 从文件名推断赛事名称（可按需改成更智能的逻辑）
	eventName := extractEventName(filePath)

	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			log.Printf("读取工作表 %s 失败: %v", sheetName, err)
			continue
		}

		for rowIdx, row := range rows {
			name, idCard, group, danceType, prize, point := extractRowData(row)

			// 跳过空行
			if name == "" || idCard == "" {
				log.Printf("警告: 文件 %s 第 %d 行身份证为空，已跳过 (姓名: %s)", filePath, rowIdx+1, name)
				continue
			}

			category := group + "-" + danceType

			// 确保 map 层级存在
			if _, ok := rankMap[category]; !ok {
				rankMap[category] = make(map[string]*model.Dancer)
			}

			dancer, exists := rankMap[category][idCard]
			if !exists {
				dancer = &model.Dancer{
					ID:     "dancer_" + utils.Generate8CharID(),
					IdCard: idCard,
					Name:   name,
					Awards: make([]model.Award, 0),
				}
				rankMap[category][idCard] = dancer
			} else {
				// 可选：姓名不一致时更新或告警
				if dancer.Name != name && name != "" {
					log.Printf("提示: 身份证 %s 姓名不一致，原: %s，新: %s", idCard, dancer.Name, name)
				}
			}

			// 追加本场成绩
			award := model.Award{
				Event: eventName + "超级赛",
				Name:  prize,
				Prize: "0",
				Group: group,
				Point: point,
			}
			dancer.Awards = append(dancer.Awards, award)
		}
	}

	return nil
}

// extractRowData 从一行中安全提取字段（集中管理列索引，方便以后改表结构）
func extractRowData(row []string) (name, idCard, group, danceType, prize, point string) {
	getVal := func(idx int) string {
		if idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	name = getVal(1)
	idCard = getVal(2)
	group, danceType = ParseGroupAndDanceType(getVal(4))
	prize = getVal(8)
	point = getVal(9)
	return
}

// ======================== 合并 / 加载已有数据 ========================

// loadExistingRanking 从 JSON 加载已有排名到 rankMap
func loadExistingRanking(jsonPath string, rankMap map[string]map[string]*model.Dancer) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	var groups []model.RankGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		return fmt.Errorf("解析 JSON 失败: %w", err)
	}

	for _, g := range groups {
		if _, ok := rankMap[g.Category]; !ok {
			rankMap[g.Category] = make(map[string]*model.Dancer)
		}
		for _, d := range g.Dancers {
			// 深拷贝一份，避免后续修改影响原数据
			dancerCopy := *d
			dancerCopy.Awards = append([]model.Award{}, d.Awards...)
			rankMap[g.Category][d.IdCard] = &dancerCopy
		}
	}

	log.Printf("已加载已有排名: %d 个组别", len(groups))
	return nil
}

// ======================== 排序与转换 ========================

// convertAndSort 把 map 转成最终的 RankGroup 切片，并按总积分降序排序
func convertAndSort(rankMap map[string]map[string]*model.Dancer) []model.RankGroup {
	var result []model.RankGroup

	for category, dancersMap := range rankMap {
		dancerList := make([]*model.Dancer, 0, len(dancersMap))
		for _, d := range dancersMap {
			dancerList = append(dancerList, d)
		}

		// 按「所有 Awards 积分总和」降序排序
		sort.Slice(dancerList, func(i, j int) bool {
			pi := calcTotalPoint(dancerList[i])
			pj := calcTotalPoint(dancerList[j])
			if pi != pj {
				return pi > pj
			}
			// 积分相同按姓名稳定排序
			return dancerList[i].Name < dancerList[j].Name
		})

		result = append(result, model.RankGroup{
			Category: category,
			Dancers:  dancerList,
		})
	}

	// 组别按名称排序，保证输出稳定
	sort.Slice(result, func(i, j int) bool {
		return result[i].Category < result[j].Category
	})

	return result
}

// calcTotalPoint 计算选手最高的 3 场赛事积分总和（不足 3 场则有几场算几场）
func calcTotalPoint(d *model.Dancer) int {
	if len(d.Awards) == 0 {
		return 0
	}

	// 1. 提取所有积分
	points := make([]int, 0, len(d.Awards))
	for _, a := range d.Awards {
		points = append(points, parsePointToInt(a.Point))
	}

	// 2. 降序排序
	sort.Sort(sort.Reverse(sort.IntSlice(points)))

	// 3. 只取前 3 个（或不足 3 个的全部）
	limit := 3
	if len(points) < limit {
		limit = len(points)
	}

	total := 0
	for i := 0; i < limit; i++ {
		total += points[i]
	}
	return total
}

// ======================== 工具函数 ========================

func ParseGroupAndDanceType(str string) (string, string) {
	str = strings.TrimSpace(str)
	parts := strings.Split(str, " ")
	if len(parts) >= 2 {
		return parts[0], strings.Join(parts[1:], " ")
	}
	return str, str
}

func parsePointToInt(pointStr string) int {
	clean := strings.ReplaceAll(strings.TrimSpace(pointStr), "分", "")
	val, err := strconv.Atoi(clean)
	if err != nil {
		return 0
	}
	return val
}

// extractEventName 从文件路径简单提取赛事名称（可按实际命名规则优化）
func extractEventName(filePath string) string {
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return name // 例如 "7月26"
}

// saveRanking 保存最终结果
func saveRanking(data []model.RankGroup, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// ============= 直通选手 ==============
func ParseDirectExcel() (map[string]*model.Dancer, error) {
	f, err := excelize.OpenFile("./data/excel/7月冠亚季总表.xlsx")
	if err != nil {
		log.Printf("打开文件失败: %v", err)
		os.Exit(-1)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("关闭文件失败: %v", err)
		}
	}()

	// 【优化点 1】：使用 Map 按身份证号聚合选手，避免同一个选手被重复创建
	dancerMap := make(map[string]*model.Dancer)

	sheetList := f.GetSheetList()
	for _, sheetName := range sheetList {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			log.Printf("读取工作表 %s 失败: %v", sheetName, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}

		// 用于向下继承的变量
		var lastGroup, lastEvent string

		for rowsIdx, row := range rows {
			if rowsIdx == 0 {
				continue // 跳过表头
			}

			// 【优化点 2】：更简洁的合并单元格继承逻辑
			if len(row) > 0 && strings.TrimSpace(row[0]) != "" {
				lastGroup = strings.TrimSpace(row[0])
			}
			if len(row) > 1 && strings.TrimSpace(row[1]) != "" {
				lastEvent = strings.TrimSpace(row[1])
			}

			// 获取当前行的值（如果为空则使用继承的值）
			currentGroup := lastGroup
			currentEvent := lastEvent

			// 辅助函数：安全获取列值，防止越界 panic
			getVal := func(idx int) string {
				if idx < len(row) {
					return strings.TrimSpace(row[idx])
				}
				return ""
			}

			name := getVal(3)   // 姓名
			idCard := getVal(6) // 身份证

			// 如果姓名和身份证都为空，说明是空白行，直接跳过
			if name == "" && idCard == "" {
				continue
			}

			// 【优化点 3】：以身份证为唯一标识聚合选手 (如果没有身份证，退而求其次用姓名)
			key := idCard
			if key == "" {
				key = name
			}

			// 如果 Map 中还没有这个选手，则初始化
			if _, exists := dancerMap[key]; !exists {
				dancerMap[key] = &model.Dancer{
					ID:     "direct_" + utils.Generate8CharID(), // 如有需要可在此生成 UUID
					IdCard: idCard,
					Name:   name,
					Awards: []model.Award{},
				}
			}
			cellRef, _ := excelize.CoordinatesToCellName(3, rowsIdx+1)
			if IsYellow(f, sheetName, cellRef) {
				dancerMap[key].IsDirect = true // 只要有颜色，就标记为 true
			}
			// 构建当前行的 Award 记录
			award := model.Award{
				Group: currentGroup,
				Event: currentEvent,
				Name:  getVal(2), // 成绩/名次
				Prize: getVal(5), // 奖金 (保留 string，如需计算可在此处用正则提取数字转 int)
			}

			// 【优化点 4】：在行循环的末尾，将 award 追加到该选手的 Awards 数组中
			dancerMap[key].Awards = append(dancerMap[key].Awards, award)
		}
	}

	// 将 Map 转换回 Slice，方便序列化或返回
	var dancers []*model.Dancer
	for _, dancer := range dancerMap {
		dancers = append(dancers, dancer)
	}

	// 打印格式化后的 JSON 查看结果
	jsonData, err := json.MarshalIndent(dancers, "", "  ")
	if err != nil {
		log.Printf("JSON 序列化失败: %v", err)
		return nil, err
	}
	outputFile := filepath.Join("./data", "direct.jsonc")

	err = os.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		log.Printf("写入 JSON 文件失败: %v", err)
		return nil, err
	}
	fmt.Printf("✅ 成功解析 %d 位选手数据，并已保存到: %s\n", len(dancers), outputFile)
	return dancerMap, nil
}

func IsYellow(f *excelize.File, sheetName, cell string) bool {
	// 2. 获取单元格的样式 ID (Style Index)
	styleID, err := f.GetCellStyle(sheetName, cell)
	if err != nil {
		log.Fatalf("获取单元格样式 ID 失败: %v", err)
	}

	// 3. 根据样式 ID 获取完整的样式定义对象
	style, err := f.GetStyle(styleID)
	if err != nil {
		log.Printf("获取样式定义失败: %v", err)
		return false
	}

	// 4. 解析并读取底色 (Fill)
	if style != nil && style.Fill.Type == "pattern" && len(style.Fill.Color) > 0 {
		// Fill.Color 是一个字符串数组，通常包含一个 6 位的 RGB 十六进制颜色值 (不带 # 号)
		log.Printf("单元格 %s 的底色是: %s\n", cell, style.Fill.Color[0])
		if style.Fill.Color[0] == "FFFF00" {
			return true
		}
	} else {
		log.Printf("单元格 %s 没有设置明确的底色 (或为默认透明/白色)\n", cell)
	}
	return false
}

// ============ 公共函数 ===========
