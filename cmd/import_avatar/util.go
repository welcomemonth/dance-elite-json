package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/meetmonth/dance-elite/internal/model"
)

func loadRanking(path string) (*model.RankingData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 ranking 失败: %w", err)
	}
	var data model.RankingData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("解析 ranking 失败: %w", err)
	}
	return &data, nil
}

func saveRanking(data *model.RankingData, path string) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// dancerRef 指向 ranking 里某个选手，方便直接改 Avatar
type dancerRef struct {
	ID     string
	Name   string
	IdCard string
	Avatar *string // 指向原结构里的 Avatar 字段
}

func buildNameIndex(data *model.RankingData) map[string][]*dancerRef {
	index := make(map[string][]*dancerRef)

	add := func(level, id, name, idCard string, avatar *string) {
		name = strings.TrimSpace(name)
		if name == "" || id == "" || level == "" {
			return
		}
		key := level + name // 关键：级别+名字 作为唯一 key
		index[key] = append(index[key], &dancerRef{
			ID:     id,
			Name:   name,
			IdCard: idCard,
			Avatar: avatar,
		})
	}

	for i := range data.Boards {
		board := &data.Boards[i]

		// ↓↓↓ 这里根据你实际的 Board 字段改，把级别提取出来
		level := extractLevel(board.Category) // 或者 board.Title / board.Category 等
		// 如果 Board 直接有 Level 字段就更简单：level := board.Level

		for j := range board.Direct {
			d := board.Direct[j]
			add(level, d.ID, d.Name, d.IdCard, &d.Avatar)
		}
		for j := range board.Ranking {
			d := board.Ranking[j]
			add(level, d.ID, d.Name, d.IdCard, &d.Avatar)
		}
	}
	return index
}

func extractLevel(s string) string {
	s = strings.ToUpper(s)
	for _, lv := range []string{"U11", "U13", "U15", "U17", "U19"} { // 按你实际有的级别加
		if strings.Contains(s, lv) {
			return lv
		}
	}
	return ""
}

func listImageFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取 images 目录失败: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".bmp":
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

// 文件名（含扩展名）中包含选手姓名即视为候选
// 文件名中同时包含 级别 和 名字 才算匹配
func findImagesContainingName(imageFiles []string, level, name string) []string {
	var matched []string
	for _, p := range imageFiles {
		base := filepath.Base(p)
		baseUpper := strings.ToUpper(base)
		if strings.Contains(baseUpper, level) && strings.Contains(base, name) {
			matched = append(matched, p)
		}
	}
	return matched
}

func processAndSaveAvatar(srcPath, dancerID string) error {
	img, err := imaging.Open(srcPath)
	if err != nil {
		return fmt.Errorf("打开图片失败: %w", err)
	}

	// 居中裁成正方形
	// w := img.Bounds().Dx()
	// h := img.Bounds().Dy()
	// side := w
	// if h < w {
	// 	side = h
	// }
	// img = imaging.CropCenter(img, side, side)

	// // 缩放到头像尺寸
	// img = imaging.Resize(img, avatarSize, avatarSize, imaging.Lanczos)

	// 只降低分辨率，不裁剪
	img = imaging.Resize(img, avatarSize, 0, imaging.Lanczos) // 宽限制为 400，高按比例
	// 或者
	// img = imaging.Resize(img, 0, avatarSize, imaging.Lanczos)  // 高限制为 400

	// 目录：uploads/avatars/{id}/
	destDir := filepath.Join(avatarsRoot, dancerID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	destPath := filepath.Join(destDir, avatarFileName)
	if err := imaging.Save(img, destPath, imaging.JPEGQuality(jpegQuality)); err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}
	return nil
}

func resizeAndCropCenter(img image.Image, size int) image.Image {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	var resized image.Image
	if w < h {
		resized = imaging.Resize(img, size, 0, imaging.Lanczos)
	} else {
		resized = imaging.Resize(img, 0, size, imaging.Lanczos)
	}
	return imaging.CropCenter(resized, size, size)
}
