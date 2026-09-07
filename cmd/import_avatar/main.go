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
		// 从 key 里拆出 level 和 name（或者你在 dancerRef 里再存一份 level）
		// 简单做法：因为 key = level + name，我们可以再调一次 extract
		if len(dancers) == 0 {
			continue
		}
		// ↓↓↓ 把这两行提前到这里
		dancer := dancers[0]
		level := extractLevel(key)
		name := dancer.Name

		if len(dancers) > 1 {
			// 用身份证号判断是不是同一个人
			idCardSet := make(map[string]bool)
			for _, d := range dancers {
				if d.IdCard != "" {
					idCardSet[d.IdCard] = true
				}
			}
			// 2. 判断身份证号是不是都一样
			if len(idCardSet) <= 1 {
				// 身份证号相同 → 视为同一个人（只是参加了多个舞种）
				log.Printf("[同人多赛] 「%s」同一级别同一名字，身份证号一致，共 %d 条记录，共用同一张头像", key, len(dancers))

				matched := findImagesContainingName(imageFiles, level, name)

				switch len(matched) {
				case 0:
					log.Printf("[未匹配到图片] 选手「%s」(id=%s) ...", name, dancer.ID)
				case 1:
					imgPath := matched[0]
					if err := processAndSaveAvatar(imgPath, dancer.ID); err != nil {
						log.Printf("[处理失败] ...")
						continue
					}
					rel := filepath.ToSlash(filepath.Join("avatars", dancer.ID, avatarFileName))

					// 关键：把这张头像赋给所有同人记录
					for _, d := range dancers {
						d.Avatar = &rel
					}
					usedImages[imgPath] = true
					log.Printf("[成功] 同人多赛「%s」共 %d 条 ← %s", key, len(dancers), filepath.Base(imgPath))
				default:
					log.Printf("[多图匹配] ...")
				}
				continue // 处理完同人后跳过后面的普通逻辑
			}
			// 3. 身份证号不一样 → 真的是不同人，跳过
			log.Printf("[重名] key「%s」对应 %d 名选手，且身份证号不同，跳过", key, len(dancers))
			for _, d := range dancers {
				log.Printf("       - id=%s  idCard=%s", d.ID, d.IdCard)
			}
			continue
		}

		matched := findImagesContainingName(imageFiles, level, name)
		// 后面 switch 逻辑完全不用动

		switch len(matched) {
		case 0:
			log.Printf("[未匹配到图片] 选手「%s」(id=%s) 在 images 中找不到包含其姓名的文件", name, dancer.ID)
		case 1:
			imgPath := matched[0]
			if err := processAndSaveAvatar(imgPath, dancer.ID); err != nil {
				log.Printf("[处理失败] 选手「%s」(id=%s) 图片 %s: %v", name, dancer.ID, imgPath, err)
				continue
			}
			// 回写 avatar 字段
			rel := filepath.ToSlash(filepath.Join("avatars", dancer.ID, avatarFileName))
			dancer.Avatar = &rel
			usedImages[imgPath] = true
			log.Printf("[成功] 选手「%s」(id=%s) ← %s → %s", name, dancer.ID, filepath.Base(imgPath), rel)
		default:
			log.Printf("[多图匹配] 选手「%s」(id=%s) 匹配到 %d 张图片，跳过，请人工确认:", name, dancer.ID, len(matched))
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
