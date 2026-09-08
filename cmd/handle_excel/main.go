package main

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/xuri/excelize/v2"
)

func main() {
	// handle_player()
	handle_institution()
}

func handle_institution() {
	filePath := "./data/excel/机构积分汇总.xlsx" // 替换为你的实际 Excel 文件路径

	f, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Fatalf("打开 Excel 文件失败: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("关闭 Excel 文件失败: %v", err)
		}
	}()

	sheetList := f.GetSheetList()
	if len(sheetList) == 0 {
		log.Fatal("Excel 文件中没有找到任何工作表")
	}

	// 用于统计每个机构的总分
	institutionPointsMap := make(map[string]int)

	// 遍历每一个工作表 (根据你的输出，这里通常只有一个"查分表格")
	for _, sheetName := range sheetList {
		rows, err := f.Rows(sheetName)
		if err != nil {
			log.Printf("读取工作表 [%s] 失败: %v\n", sheetName, err)
			continue
		}

		rowNum := 1
		for rows.Next() {
			columns, err := rows.Columns()
			if err != nil {
				log.Printf("读取第 %d 行数据失败: %v\n", rowNum, err)
				rowNum++
				continue
			}

			// 1. 跳过表头 (假设第一行包含 "序号" 或 "所属机构")
			if rowNum == 1 || (len(columns) > 0 && columns[0] == "序号") {
				rowNum++
				continue
			}

			// 2. 防御性编程：确保该行至少有 10 列，避免数组越界 panic
			if len(columns) < 4 {
				rowNum++
				continue
			}

			// 3. 提取关键列 (索引从 0 开始)
			// 列 8: 所属机构 (索引 7)
			// 列 10: 本站积分 (索引 9)
			institutionName := strings.TrimSpace(columns[1])
			pointsStr := strings.TrimSpace(columns[4])
			if pointsStr == "" {
				cellRef, _ := excelize.CoordinatesToCellName(5, rowNum)
				calculated, err := f.CalcCellValue(sheetName, cellRef)
				if err != nil {
					log.Printf("警告: 文件 %s 第 %d 行公式计算失败: %v，按 0 处理", filePath, rowNum, err)
				} else {
					pointsStr = strings.TrimSpace(calculated)
				}
			}
			// 如果机构名称为空，则跳过
			if institutionName == "" {
				rowNum++
				continue
			}

			// 4. 将积分字符串转换为整数
			points := 0
			if pointsStr != "" {
				parsedPoints, err := strconv.Atoi(pointsStr)
				if err != nil {
					log.Printf("警告: 第 %d 行积分 '%s' 无法转换为整数，按 0 分处理\n", rowNum, pointsStr)
				} else {
					points = parsedPoints
				}
			}

			// 5. 累加到 map 中
			institutionPointsMap[institutionName] += points

			rowNum++
		}

		if err := rows.Close(); err != nil {
			log.Printf("关闭行迭代器失败: %v\n", err)
		}
	}

	// 6. 将 map 转换为切片，以便进行排序
	var institutions []model.Institution
	for name, pts := range institutionPointsMap {
		institutions = append(institutions, model.Institution{
			Name:   name,
			Points: pts,
		})
	}

	// 7. 按总分降序排序 (如果分数相同，则按机构名称字典序升序排列，保证排序稳定性)
	sort.Slice(institutions, func(i, j int) bool {
		if institutions[i].Points == institutions[j].Points {
			return institutions[i].Name < institutions[j].Name
		}
		return institutions[i].Points > institutions[j].Points
	})

	// 8. 封装到最终的结构体中
	ranking := model.InstitutionRanking{
		Institutions: institutions,
	}

	// 5. 保存到 JSON
	if err := saveInstitutionRanking(institutionJSONPath, &ranking); err != nil {
		log.Printf("保存机构排行出错：%v", err)
		return
	}
	fmt.Printf("结果已保存到: %s\n", institutionJSONPath)
}

func handle_player() {
	// 首次生成
	// err := ranking.ProcessAll(
	// 	[]string{"./data/excel/7月26.xlsx"},
	// 	"./data/excel/7月冠亚季总表.xlsx",
	// 	"",    // 无历史
	// 	false, // 非合并
	// )
	// 后续合并
	err := ProcessAll(
		[]string{"./data/excel/7月26日超级赛.xlsx"}, // 新积分文件
		"./data/excel/7月冠亚季总表.xlsx",            // 直通文件
		"./data/ranking.json",                  // 已有结果
		true,                                   // 合并模式
	)
	if err != nil {
		log.Fatal(err)
	}
}
