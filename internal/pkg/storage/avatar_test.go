package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestEncodeAvatarScalesAndFlattensAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1000, 500))
	for i := range src.Pix {
		src.Pix[i] = 0 // 全透明像素
	}
	data, err := EncodeAvatar(src, 512)
	if err != nil {
		t.Fatalf("EncodeAvatar: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode avatar jpeg: %v", err)
	}
	if img.Bounds().Dx() != 512 || img.Bounds().Dy() != 256 {
		t.Fatalf("avatar bounds = %v, want 512x256", img.Bounds())
	}
	// 透明背景合成白色（JPEG 无 alpha）。
	if r, g, b, _ := img.At(10, 10).RGBA(); r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatalf("transparent pixel not flattened to white: %v %v %v", r, g, b)
	}
}

func TestEncodeAvatarKeepsSmallImageSize(t *testing.T) {
	small := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			small.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	data, err := EncodeAvatar(small, 512)
	if err != nil {
		t.Fatalf("EncodeAvatar small: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode small avatar: %v", err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 80 {
		t.Fatalf("small avatar bounds = %v, want 100x80", img.Bounds())
	}
}
