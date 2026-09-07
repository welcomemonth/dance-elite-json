package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/utils"
	"github.com/xuri/excelize/v2"
)

// ======================== 配置 ========================

const (
	defaultOutputDir  = "./data"
	defaultOutputFile = "ranking.json"
)

// 固定 6 个榜单，保证输出顺序稳定
var fixedBoards = []struct {
	Level     string
	DanceType string
}{
	{"U11", "古典舞"},
	{"U11", "民族民间舞"},
	{"U13", "古典舞"},
	{"U13", "民族民间舞"},
	{"U15", "古典舞"},
	{"U15", "民族民间舞"},
}

// 组别解析正则：U11/U13/U15 + 可选空格 + 古典舞/民族民间舞
var categoryRe = regexp.MustCompile(`^(U1[135])\s*(古典舞|民族民间舞)$`)

// ======================== 入口 ========================

// ProcessAll 统一入口
// pointsExcelFiles: 积分 Excel 路径列表
// directExcelPath:  直通 Excel 路径
// existingJSON:     已有 ranking.json（合并时用，空字符串表示不加载）
// mergeMode:        true=合并历史，false=仅用本次数据
func ProcessAll(pointsExcelFiles []string, directExcelPath, existingJSON string, mergeMode bool) error {
	// ---------- Phase 0: 准备 ----------
	boards := initEmptyBoards()

	oldPointsMap := make(map[string]map[string]*model.RankDancer)   // category -> idCard
	oldDirectMap := make(map[string]map[string]*model.DirectDancer) // category -> idCard
	oldRankMap := make(map[string]map[string]int)                   // category -> idCard -> rank

	if mergeMode && existingJSON != "" {
		if err := loadExisting(existingJSON, oldPointsMap, oldDirectMap, oldRankMap); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("加载已有排名失败: %w", err)
			}
			log.Printf("提示: 已有排名文件不存在，按首次生成处理")
		}
	}

	// ---------- Phase 1: 解析积分 Excel ----------
	pointsMap := make(map[string]map[string]*model.Dancer) // category -> idCard
	for _, filePath := range pointsExcelFiles {
		if err := parsePointsExcel(filePath, pointsMap, oldPointsMap, oldDirectMap); err != nil {
			return fmt.Errorf("解析积分文件 %s 失败: %w", filePath, err)
		}
	}
	if mergeMode {
		mergeOldPointsIntoMap(pointsMap, oldPointsMap)
	}

	// ---------- Phase 2: 解析直通 Excel ----------
	directMap := make(map[string]map[string]*model.Dancer) // category -> idCard
	if directExcelPath != "" {
		if err := parseDirectExcel(directExcelPath, directMap, oldDirectMap); err != nil {
			return fmt.Errorf("解析直通文件失败: %w", err)
		}
	}

	// ---------- Phase 3: 组装 ----------
	assembleBoards(boards, pointsMap, directMap, oldRankMap)

	// ---------- Phase 4: 写出 ----------
	data := model.RankingData{
		Version:   time.Now().Format("2006-01-02"),
		UpdatedAt: time.Now().Format(time.RFC3339),
		Boards:    boards,
	}

	outputPath := filepath.Join(defaultOutputDir, defaultOutputFile)
	if err := saveRanking(data, outputPath); err != nil {
		return err
	}

	fmt.Printf("✅ 处理完成，共 %d 个榜单，已保存到: %s\n", len(data.Boards), outputPath)
	return nil
}

// 把历史积分选手灌进 pointsMap（仅 merge 时调用）
func mergeOldPointsIntoMap(
	pointsMap map[string]map[string]*model.Dancer,
	oldPointsMap map[string]map[string]*model.RankDancer,
) {
	for cat, oldDancers := range oldPointsMap {
		if _, ok := pointsMap[cat]; !ok {
			pointsMap[cat] = make(map[string]*model.Dancer)
		}
		for idCard, old := range oldDancers {
			if _, exists := pointsMap[cat][idCard]; exists {
				continue // 新文件已有此人，留给下面追加 Awards
			}
			// 旧人原样带过来
			pointsMap[cat][idCard] = &model.Dancer{
				ID:     old.ID,
				Name:   old.Name,
				Avatar: old.Avatar,
				IdCard: old.IdCard,
				Awards: append([]model.Award{}, old.Awards...), // 深拷贝
			}
		}
	}
}

// ======================== Phase 0 ========================

func initEmptyBoards() []model.Board {
	boards := make([]model.Board, 0, len(fixedBoards))
	for _, b := range fixedBoards {
		boards = append(boards, model.Board{
			Category:  b.Level + "-" + b.DanceType,
			Level:     b.Level,
			DanceType: b.DanceType,
			Direct:    []*model.DirectDancer{},
			Ranking:   []*model.RankDancer{},
		})
	}
	return boards
}

func loadExisting(
	jsonPath string,
	oldPointsMap map[string]map[string]*model.RankDancer,
	oldDirectMap map[string]map[string]*model.DirectDancer,
	oldRankMap map[string]map[string]int,
) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	var ranking model.RankingData
	if err := json.Unmarshal(data, &ranking); err != nil {
		return fmt.Errorf("解析 JSON 失败: %w", err)
	}

	for _, board := range ranking.Boards {
		cat := board.Category

		if _, ok := oldPointsMap[cat]; !ok {
			oldPointsMap[cat] = make(map[string]*model.RankDancer)
			oldRankMap[cat] = make(map[string]int)
		}
		for _, d := range board.Ranking {
			// 深拷贝 Awards
			cp := *d
			cp.Awards = append([]model.Award{}, d.Awards...)
			oldPointsMap[cat][d.IdCard] = &cp
			oldRankMap[cat][d.IdCard] = d.Rank
		}

		if _, ok := oldDirectMap[cat]; !ok {
			oldDirectMap[cat] = make(map[string]*model.DirectDancer)
		}
		for _, d := range board.Direct {
			cp := *d
			cp.Awards = append([]model.Award{}, d.Awards...)
			oldDirectMap[cat][d.IdCard] = &cp
		}
	}

	log.Printf("已加载历史排名: %d 个榜单", len(ranking.Boards))
	return nil
}

// ======================== Phase 1: 积分 Excel ========================

func parsePointsExcel(
	filePath string,
	pointsMap map[string]map[string]*model.Dancer,
	oldPointsMap map[string]map[string]*model.RankDancer,
	oldDirectMap map[string]map[string]*model.DirectDancer,
) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	eventName := extractEventName(filePath)

	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			log.Printf("读取工作表 %s 失败: %v", sheetName, err)
			continue
		}

		for rowIdx, row := range rows {
			name, idCard, groupRaw, prize, point := extractPointsRow(row)
			if name == "" || idCard == "" {
				if name != "" {
					log.Printf("警告: %s 第 %d 行身份证为空，已跳过 (姓名: %s)", filePath, rowIdx+1, name)
				}
				continue
			}

			_, _, category := ParseCategory(groupRaw)
			if category == "" {
				log.Printf("警告: 无法解析组别 [%s]，跳过", groupRaw)
				continue
			}

			if _, ok := pointsMap[category]; !ok {
				pointsMap[category] = make(map[string]*model.Dancer)
			}

			dancer, exists := pointsMap[category][idCard]
			if !exists {
				// 优先复用历史 ID / Avatar
				id, avatar := reuseIDAndAvatar(idCard, category, oldPointsMap, oldDirectMap)
				// ★ 新增：把历史 Awards 带过来
				var oldAwards []model.Award
				if m, ok := oldPointsMap[category]; ok {
					if old, ok := m[idCard]; ok {
						oldAwards = append([]model.Award{}, old.Awards...)
					}
				}
				dancer = &model.Dancer{
					ID:     id,
					Name:   name,
					Avatar: avatar,
					IdCard: idCard,
					Awards: oldAwards,
				}
				pointsMap[category][idCard] = dancer
			} else if dancer.Name != name && name != "" {
				log.Printf("提示: 身份证 %s 姓名不一致，原: %s，新: %s", idCard, dancer.Name, name)
				dancer.Name = name
			}

			dancer.Awards = append(dancer.Awards, model.Award{
				Event: eventName,
				Name:  prize,
				Prize: "0",
				Group: groupRaw,
				Point: point,
			})
		}
	}
	return nil
}

func extractPointsRow(row []string) (name, idCard, groupRaw, prize, point string) {
	getVal := func(idx int) string {
		if idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}
	// 按你现有列索引，如有变化只改这里
	name = getVal(1)
	idCard = getVal(2)
	groupRaw = getVal(4) // 例如 "U11古典舞"
	prize = getVal(8)
	point = getVal(9)
	return
}

// ======================== Phase 2: 直通 Excel ========================

func parseDirectExcel(
	filePath string,
	directMap map[string]map[string]*model.Dancer,
	oldDirectMap map[string]map[string]*model.DirectDancer,
) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			log.Printf("读取工作表 %s 失败: %v", sheetName, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}

		var lastGroup, lastEvent string

		for rowIdx, row := range rows {
			if rowIdx == 0 {
				continue // 跳过表头
			}

			// 合并单元格继承
			if len(row) > 0 && strings.TrimSpace(row[0]) != "" {
				lastGroup = strings.TrimSpace(row[0])
			}
			if len(row) > 1 && strings.TrimSpace(row[1]) != "" {
				lastEvent = strings.TrimSpace(row[1])
			}

			getVal := func(idx int) string {
				if idx < len(row) {
					return strings.TrimSpace(row[idx])
				}
				return ""
			}

			name := getVal(3)
			idCard := getVal(6)
			if name == "" && idCard == "" {
				continue
			}

			// 判断是否直通（黄色）
			cellRef, _ := excelize.CoordinatesToCellName(4, rowIdx+1) // 姓名列，按你原来是列3 → 1-based 是 4
			// 注意：你原来写的是 CoordinatesToCellName(3, ...)，请按实际姓名列调整
			if !IsYellow(f, sheetName, cellRef) {
				continue
			}

			_, _, category := ParseCategory(lastGroup)
			if category == "" {
				log.Printf("警告: 直通行无法解析组别 [%s]，姓名: %s", lastGroup, name)
				continue
			}

			if _, ok := directMap[category]; !ok {
				directMap[category] = make(map[string]*model.Dancer)
			}

			key := idCard
			if key == "" {
				key = name
			}

			dancer, exists := directMap[category][key]
			if !exists {
				id, avatar := "", ""
				if old, ok := oldDirectMap[category][idCard]; ok {
					id = old.ID
					avatar = old.Avatar
				}
				if id == "" {
					id = "direct_" + utils.Generate8CharID()
				}
				dancer = &model.Dancer{
					ID:     id,
					Name:   name,
					Avatar: avatar,
					IdCard: idCard,
					Awards: make([]model.Award, 0),
				}
				directMap[category][key] = dancer
			}

			award := model.Award{
				Group: lastGroup,
				Event: lastEvent,
				Name:  getVal(2), // 成绩/名次
				Prize: getVal(5), // 奖金
			}
			dancer.Awards = append(dancer.Awards, award)
		}
	}
	return nil
}

// ======================== Phase 3: 组装 ========================

func assembleBoards(
	boards []model.Board,
	pointsMap map[string]map[string]*model.Dancer,
	directMap map[string]map[string]*model.Dancer,
	oldRankMap map[string]map[string]int,
) {
	for i := range boards {
		board := &boards[i]
		cat := board.Category

		// 3.1 直通列表
		directIdSet := make(map[string]struct{})
		if dm, ok := directMap[cat]; ok {
			for _, d := range dm {
				board.Direct = append(board.Direct, &model.DirectDancer{
					ID:     d.ID,
					Name:   d.Name,
					Avatar: d.Avatar,
					IdCard: d.IdCard,
					Awards: d.Awards,
				})
				if d.IdCard != "" {
					directIdSet[d.IdCard] = struct{}{}
				}
			}
		}

		// 3.2 积分候选人（排除直通）
		type candidate struct {
			dancer *model.Dancer
			points int
		}
		var candidates []candidate

		if pm, ok := pointsMap[cat]; ok {
			for _, p := range pm {
				if _, isDirect := directIdSet[p.IdCard]; isDirect {
					continue
				}
				pts := calcTotalPoint(p.Awards)
				candidates = append(candidates, candidate{dancer: p, points: pts})
			}
		}

		// 3.3 排序
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].points != candidates[j].points {
				return candidates[i].points > candidates[j].points
			}
			return candidates[i].dancer.Name < candidates[j].dancer.Name
		})

		// 3.4 生成 Rank / PrevRank / RankChange
		for idx, c := range candidates {
			rank := idx + 1
			prevRank := 0
			if rm, ok := oldRankMap[cat]; ok {
				prevRank = rm[c.dancer.IdCard]
			}
			change := 0
			if prevRank > 0 {
				change = prevRank - rank
			}

			board.Ranking = append(board.Ranking, &model.RankDancer{
				ID:         c.dancer.ID,
				Rank:       rank,
				PrevRank:   prevRank,
				RankChange: change,
				Name:       c.dancer.Name,
				Avatar:     c.dancer.Avatar,
				IdCard:     c.dancer.IdCard,
				Points:     c.points,
				Awards:     c.dancer.Awards,
			})
		}
	}
}

// ======================== 工具函数 ========================

// ParseCategory 解析 "U11古典舞" / "U13 民族民间舞" → level, danceType, category
func ParseCategory(raw string) (level, danceType, category string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}

	if m := categoryRe.FindStringSubmatch(raw); len(m) == 3 {
		level = m[1]
		danceType = m[2]
		category = level + "-" + danceType
		return
	}

	// 兜底：前 3 字符当级别
	if len(raw) >= 3 {
		level = raw[:3]
		danceType = strings.TrimSpace(raw[3:])
		if danceType != "" {
			category = level + "-" + danceType
			return
		}
	}
	return "", "", ""
}

func calcTotalPoint(awards []model.Award) int {
	if len(awards) == 0 {
		return 0
	}
	points := make([]int, 0, len(awards))
	for _, a := range awards {
		points = append(points, parsePointToInt(a.Point))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(points)))

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

func parsePointToInt(pointStr string) int {
	clean := strings.ReplaceAll(strings.TrimSpace(pointStr), "分", "")
	val, err := strconv.Atoi(clean)
	if err != nil {
		return 0
	}
	return val
}

func extractEventName(filePath string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func reuseIDAndAvatar(
	idCard, category string,
	oldPointsMap map[string]map[string]*model.RankDancer,
	oldDirectMap map[string]map[string]*model.DirectDancer,
) (id, avatar string) {
	if m, ok := oldPointsMap[category]; ok {
		if old, ok := m[idCard]; ok {
			return old.ID, old.Avatar
		}
	}
	if m, ok := oldDirectMap[category]; ok {
		if old, ok := m[idCard]; ok {
			return old.ID, old.Avatar
		}
	}
	return "dancer_" + utils.Generate8CharID(), ""
}

func saveRanking(data model.RankingData, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}
	return os.WriteFile(outputPath, jsonData, 0644)
}

// IsYellow 保持你原来的实现（判断单元格是否黄色）
func IsYellow(f *excelize.File, sheetName, cell string) bool {
	styleID, err := f.GetCellStyle(sheetName, cell)
	if err != nil {
		log.Printf("获取单元格样式 ID 失败: %v", err)
		return false
	}
	style, err := f.GetStyle(styleID)
	if err != nil {
		log.Printf("获取样式定义失败: %v", err)
		return false
	}
	if style != nil && style.Fill.Type == "pattern" && len(style.Fill.Color) > 0 {
		if style.Fill.Color[0] == "FFFF00" {
			return true
		}
	}
	return false
}
