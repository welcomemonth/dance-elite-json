package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/meetmonth/dance-elite/internal/model"
)

const institutionJSONPath = "./data/institution.json"

// loadInstitutionRanking 从 JSON 文件加载已有排行榜，文件不存在时返回空结构
func loadInstitutionRanking(path string) (*model.InstitutionRanking, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.InstitutionRanking{Institutions: []model.Institution{}}, nil
		}
		return nil, fmt.Errorf("读取 JSON 失败: %w", err)
	}

	var ranking model.InstitutionRanking
	if err := json.Unmarshal(data, &ranking); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}
	return &ranking, nil
}

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
