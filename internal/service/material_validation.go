package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	// 注册 jpeg/png 解码器供 image.Decode/DecodeConfig 使用。
	_ "image/jpeg"
	_ "image/png"

	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 参赛资料上传限制（API 文档 4.1，开发计划 T13）。
const (
	MaxPhotoBytes = 10 << 20  // 证件照 png/jpg ≤10MB
	MaxAudioBytes = 100 << 20 // 音频 mp3 ≤100MB
	// MaxMaterialRequestBytes 是请求体上限：音频 + 证件照 + multipart 边界开销。
	MaxMaterialRequestBytes = MaxAudioBytes + MaxPhotoBytes + (4 << 20)

	MaxAudioDuration = 4 * time.Minute
	AvatarMaxSide    = 512

	// maxPhotoPixels 防御超大尺寸图片（解压炸弹）：4000 万像素足以覆盖证件照。
	maxPhotoPixels = 40_000_000
)

// phonePattern 是大陆手机号格式（11 位，1 开头）。
var phonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

// materialTextPatch 是文本资料的 write patch（nil = 不修改；空串 = 清空）。
type materialTextPatch struct {
	ProgramName    *string
	TeacherName    *string
	ContactPhone   *string
	ExpressAddress *string
	WorkIntro      *string
	SpecialReq     *string
}

// validateText 校验并规范化文本字段。返回 changed=false 表示本次未提交任何文本项。
func (s *MaterialService) validateText(ctx context.Context, in MaterialUpdateInput) (materialTextPatch, bool, error) {
	var patch materialTextPatch
	provided := in.ProgramName != nil || in.TeacherName != nil || in.ContactPhone != nil ||
		in.ExpressAddress != nil || in.WorkIntro != nil || in.SpecialReq != nil
	if !provided {
		return patch, false, nil
	}

	words, err := s.sensitiveWords(ctx)
	if err != nil {
		return patch, false, httpx.ErrInternal(err, "加载内容校验配置失败，请稍后重试")
	}

	type field struct {
		value   *string
		label   string
		max     int
		isPhone bool
		dest    **string
	}
	fields := []field{
		{value: in.ProgramName, label: "节目名称", max: 50, dest: &patch.ProgramName},
		{value: in.TeacherName, label: "指导教师", max: 30, dest: &patch.TeacherName},
		{value: in.ContactPhone, label: "联系电话", isPhone: true, dest: &patch.ContactPhone},
		{value: in.ExpressAddress, label: "快递地址", max: 120, dest: &patch.ExpressAddress},
		{value: in.WorkIntro, label: "作品说明", max: 500, dest: &patch.WorkIntro},
		{value: in.SpecialReq, label: "特殊要求", max: 200, dest: &patch.SpecialReq},
	}
	for _, f := range fields {
		if f.value == nil {
			continue
		}
		v := strings.TrimSpace(*f.value)
		if f.max > 0 && utf8.RuneCountInString(v) > f.max {
			return patch, false, httpx.ErrBadRequest(fmt.Sprintf("%s不能超过 %d 字", f.label, f.max))
		}
		if f.isPhone && v != "" && !phonePattern.MatchString(v) {
			return patch, false, httpx.ErrBadRequest("联系电话格式不正确（11 位手机号）")
		}
		if v != "" && containsSensitive(v, words) {
			return patch, false, httpx.ErrBadRequest(f.label + "包含不允许的内容")
		}
		*f.dest = &v
	}
	return patch, true, nil
}

// sensitiveWords 读取可选的敏感词配置（settings.sensitive_words，JSON 字符串数组）。
// 未配置时不做过滤；配置损坏视为服务端配置错误。
func (s *MaterialService) sensitiveWords(ctx context.Context) ([]string, error) {
	raw, err := s.stores.Settings.Get(ctx, "sensitive_words")
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var words []string
	if err := json.Unmarshal([]byte(raw), &words); err != nil {
		return nil, fmt.Errorf("parse sensitive_words settings: %w", err)
	}
	return words, nil
}

// validatePhoto 校验证件照扩展名、大小与真实图片内容，返回规范扩展名与解码后的图片。
func validatePhoto(f *MaterialFile) (string, image.Image, error) {
	if f == nil || f.Content == nil || f.Size <= 0 {
		return "", nil, httpx.ErrBadRequest("证件照文件为空")
	}
	if f.Size > MaxPhotoBytes {
		return "", nil, httpx.ErrBadRequest("证件照不能超过 10MB")
	}

	ext := strings.ToLower(path.Ext(strings.ReplaceAll(f.Filename, "\\", "/")))
	var wantFormat, outExt string
	switch ext {
	case ".jpg", ".jpeg":
		wantFormat, outExt = "jpeg", "jpg"
	case ".png":
		wantFormat, outExt = "png", "png"
	default:
		return "", nil, httpx.ErrBadRequest("证件照仅支持 png/jpg 格式")
	}

	cfg, format, err := image.DecodeConfig(f.Content)
	if err != nil {
		return "", nil, httpx.ErrBadRequest("证件照文件格式不合法，请重新上传")
	}
	if format != wantFormat {
		return "", nil, httpx.ErrBadRequest("证件照文件内容与扩展名不一致，请重新上传")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxPhotoPixels {
		return "", nil, httpx.ErrBadRequest("证件照尺寸不合法，请重新上传")
	}

	if _, err := f.Content.Seek(0, io.SeekStart); err != nil {
		return "", nil, httpx.ErrInternal(err, "读取证件照失败，请重试")
	}
	img, decodedFormat, err := image.Decode(f.Content)
	if err != nil || decodedFormat != wantFormat {
		return "", nil, httpx.ErrBadRequest("证件照文件格式不合法，请重新上传")
	}
	return outExt, img, nil
}

// validateAudio 校验音频扩展名、大小与 MP3 帧头解析出的时长（R5：解析失败一律拒绝）。
func validateAudio(f *MaterialFile) error {
	if f == nil || f.Content == nil || f.Size <= 0 {
		return httpx.ErrBadRequest("音频文件为空")
	}
	if f.Size > MaxAudioBytes {
		return httpx.ErrBadRequest("音频不能超过 100MB")
	}
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(f.Filename, "\\", "/")))
	if ext != ".mp3" {
		return httpx.ErrBadRequest("音频仅支持 mp3 格式")
	}
	duration, err := utils.MP3Duration(f.Content, f.Size)
	if err != nil {
		return httpx.ErrBadRequest("音频文件格式不合法，请上传有效的 mp3 文件")
	}
	if duration > MaxAudioDuration {
		return httpx.ErrBadRequest("音频时长不能超过 4 分钟")
	}
	return nil
}

// containsSensitive 检查文本是否包含任一敏感词（忽略大小写）。
func containsSensitive(text string, words []string) bool {
	if len(words) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" && strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
