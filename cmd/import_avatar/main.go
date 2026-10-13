package main

import (
	"log"
	"path/filepath"
)

const (
	rankingPath    = "./data/ranking.json"
	imagesDir      = "./data/images"
	avatarsRoot    = "./uploads/avatars"
	avatarFileName = "avatar.jpg"
	avatarSize     = 400
	jpegQuality    = 85
)

func main() {
	if err := ImportAvatars(); err != nil {
		log.Fatal(err)
	}
}

func ImportAvatars() error {
	// 1. 读取 ranking.json
	data, err := loadRanking(rankingPath)
	if err != nil {
		return err
	}

	// 2. 建立 姓名 → 选手列表 索引（可能重名）
	nameIndex := buildNameIndex(data)

	// 3. 扫描所有图片
	imageFiles, err := listImageFiles(imagesDir)
	if err != nil {
		return err
	}
	if len(imageFiles) == 0 {
		log.Println("提示: images 目录下没有图片")
		return nil
	}

	// 记录：哪些图片已被唯一匹配使用
	usedImages := make(map[string]bool)

	// 4. 按选手姓名去匹配图片
	for key, dancers := range nameIndex { // key 现在是 "U11张三"
		if len(dancers) == 0 {
			continue
		}

		level := extractLevel(key)
		name := dancers[0].Name

		// 重名但身份证号不同 → 不是同一个人，跳过请人工确认
		if len(dancers) > 1 && !samePerson(dancers) {
			log.Printf("[重名] key「%s」对应 %d 名选手，且身份证号不同，跳过", key, len(dancers))
			for _, d := range dancers {
				log.Printf("       - id=%s  idCard=%s", d.ID, d.IdCard)
			}
			continue
		}

		// 同人多赛：同一人参加多个舞种，共用同一张头像
		if len(dancers) > 1 {
			log.Printf("[同人多赛] 「%s」同一级别同一名字，身份证号一致，共 %d 条记录，共用同一张头像", key, len(dancers))
		}

		matched := findImagesContainingName(imageFiles, level, name)

		switch len(matched) {
		case 0:
			log.Printf("[未匹配到图片] 选手「%s」在 images 中找不到包含其姓名的文件", name)
		case 1:
			imgPath := matched[0]
			// 头像只处理一次，再复制到每个选手各自的 ID 目录
			if err := assignAvatarToDancers(imgPath, dancers); err != nil {
				log.Printf("[处理失败] 图片 %s: %v", imgPath, err)
				continue
			}
			usedImages[imgPath] = true
			log.Printf("[成功] 「%s」共 %d 条 ← %s", key, len(dancers), filepath.Base(imgPath))
		default:
			log.Printf("[多图匹配] 选手「%s」匹配到 %d 张图片，跳过，请人工确认:", name, len(matched))
			for _, p := range matched {
				log.Printf("       - %s", filepath.Base(p))
			}
		}
	}

	// 5. 提示：没有任何选手匹配上的图片
	for _, imgPath := range imageFiles {
		if !usedImages[imgPath] {
			log.Printf("[图片未使用] %s 未能唯一匹配到任何选手", filepath.Base(imgPath))
		}
	}

	// 6. 写回 ranking.json
	if err := saveRanking(data, rankingPath); err != nil {
		return err
	}
	log.Println("✅ 头像导入完成，ranking.json 已更新")
	return nil
}
