package service

import (
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

func materialStrPtr(s string) *string { return &s }

func TestMaterialStatusOf(t *testing.T) {
	empty := &model.Registration{}
	status := MaterialStatusOf(empty)
	if status.Total != model.MaterialTotalFields || status.Completed != 0 {
		t.Fatalf("empty status = %+v", status)
	}
	if len(status.Missing) != model.MaterialTotalFields {
		t.Fatalf("empty missing = %v", status.Missing)
	}

	// 空白字符串不算已填写。
	blank := &model.Registration{ProgramName: materialStrPtr("   ")}
	if got := MaterialStatusOf(blank).Completed; got != 0 {
		t.Fatalf("blank program completed = %d, want 0", got)
	}

	full := &model.Registration{
		PhotoPath:      materialStrPtr("uploads/1/P001/photo_x.jpg"),
		AudioPath:      materialStrPtr("uploads/1/P001/audio_x.mp3"),
		ProgramName:    materialStrPtr("春闺梦"),
		TeacherName:    materialStrPtr("王老师"),
		ContactPhone:   materialStrPtr("13812345678"),
		ExpressAddress: materialStrPtr("上海市"),
		WorkIntro:      materialStrPtr("作品说明"),
		SpecialReq:     materialStrPtr("无"),
	}
	status = MaterialStatusOf(full)
	if status.Completed != model.MaterialTotalFields || len(status.Missing) != 0 {
		t.Fatalf("full status = %+v", status)
	}
}

func TestMaterialsSnapshotOf(t *testing.T) {
	reg := &model.Registration{
		PhotoPath:         materialStrPtr("uploads/12/P0001/photo_ab.jpg"),
		AudioPath:         materialStrPtr("uploads/12/P0001/audio_cd.mp3"),
		ProgramName:       materialStrPtr("春闺梦"),
		MaterialUpdatedAt: materialStrPtr("2026-11-11T10:00:00+08:00"),
	}
	snap := MaterialsSnapshotOf(reg)
	if snap.PhotoURL == nil || *snap.PhotoURL != "/static/materials/12/P0001/photo_ab.jpg" {
		t.Fatalf("photo_url = %v", snap.PhotoURL)
	}
	if snap.AudioURL == nil || *snap.AudioURL != "/static/materials/12/P0001/audio_cd.mp3" {
		t.Fatalf("audio_url = %v", snap.AudioURL)
	}
	if snap.AudioFilename == nil || *snap.AudioFilename != "audio_cd.mp3" {
		t.Fatalf("audio_filename = %v", snap.AudioFilename)
	}
	if snap.UpdatedAt == nil || *snap.UpdatedAt != *reg.MaterialUpdatedAt {
		t.Fatalf("updated_at = %v", snap.UpdatedAt)
	}

	empty := MaterialsSnapshotOf(&model.Registration{})
	if empty.PhotoURL != nil || empty.AudioURL != nil || empty.AudioFilename != nil || empty.UpdatedAt != nil {
		t.Fatalf("empty snapshot = %+v", empty)
	}
}

func TestPublicURLMapsAvatarAndOpaquePath(t *testing.T) {
	if got := publicURL(materialStrPtr("uploads/avatars/P0001/avatar_ab.jpg")); got == nil || *got != "/static/avatars/P0001/avatar_ab.jpg" {
		t.Fatalf("avatar url = %v", got)
	}
	// 未知前缀原样返回（未来 CDN 绝对地址）。
	if got := publicURL(materialStrPtr("https://cdn.example.com/a.jpg")); got == nil || *got != "https://cdn.example.com/a.jpg" {
		t.Fatalf("absolute url = %v", got)
	}
}
