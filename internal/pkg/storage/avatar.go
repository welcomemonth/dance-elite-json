package storage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"

	"github.com/disintegration/imaging"
)

// EncodeAvatar 生成最长边不超过 maxSide 的 JPEG 压缩副本（R8/Q23 定稿）。
// 透明背景合成白色，保证 PNG 证件照转 JPEG 不出现黑底。
func EncodeAvatar(img image.Image, maxSide int) ([]byte, error) {
	if img == nil {
		return nil, errors.New("nil image")
	}
	if maxSide <= 0 {
		maxSide = 512
	}
	bounds := img.Bounds()
	if bounds.Dx() > maxSide || bounds.Dy() > maxSide {
		img = imaging.Fit(img, maxSide, maxSide, imaging.Lanczos)
		bounds = img.Bounds()
	}

	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), img, bounds.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode avatar jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
