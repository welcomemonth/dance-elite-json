package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/meetmonth/dance-elite/internal/model"
)

const institutionJSONPath = "./data/institution.json"

// saveInstitutionRanking 将排行榜以格式化 JSON 保存到文件
func saveInstitutionRanking(path string, ranking *model.InstitutionRanking) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(ranking, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 JSON 失败: %w", err)
	}
	return nil
}
