package main

import (
	"log"
)

func main() {
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
