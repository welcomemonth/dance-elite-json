package service

import (
	"context"
	"image"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// ---- 文本字段 ----

func TestMaterialUpdate_TextFieldsAndClear(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-text")
	regID, _, _, _ := mustActiveRegistration(t, f, userID, "key-material-text1")

	program, teacher, phone := "春闺梦", "王老师", "13812345678"
	resp, err := f.svc.UpdateText(ctx, userID, regID, MaterialTextInput{
		ProgramName: &program, TeacherName: &teacher, ContactPhone: &phone,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if resp.MaterialStatus.Total != 8 || resp.MaterialStatus.Completed != 3 {
		t.Fatalf("material_status = %+v, want 3/8", resp.MaterialStatus)
	}
	if len(resp.MaterialStatus.Missing) != 5 || resp.MaterialStatus.Missing[0] != model.MaterialFieldPhoto {
		t.Fatalf("missing = %v", resp.MaterialStatus.Missing)
	}
	if resp.Materials.ProgramName == nil || *resp.Materials.ProgramName != program {
		t.Fatalf("program_name = %v", resp.Materials.ProgramName)
	}
	if resp.Materials.UpdatedAt == nil {
		t.Fatal("updated_at is nil after update")
	}

	var dbProgram, dbPhone *string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT program_name, contact_phone FROM registrations WHERE id = ?`, regID).Scan(&dbProgram, &dbPhone); err != nil {
		t.Fatalf("query registration: %v", err)
	}
	if dbProgram == nil || *dbProgram != program || dbPhone == nil || *dbPhone != phone {
		t.Fatalf("db program=%v phone=%v", dbProgram, dbPhone)
	}

	// 显式传空串清空字段。
	empty := ""
	resp, err = f.svc.UpdateText(ctx, userID, regID, MaterialTextInput{ProgramName: &empty})
	if err != nil {
		t.Fatalf("clear program: %v", err)
	}
	if resp.Materials.ProgramName != nil {
		t.Fatalf("program_name after clear = %v, want nil", *resp.Materials.ProgramName)
	}
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT program_name FROM registrations WHERE id = ?`, regID).Scan(&dbProgram); err != nil {
		t.Fatalf("query cleared program: %v", err)
	}
	if dbProgram != nil {
		t.Fatalf("db program after clear = %q, want NULL", *dbProgram)
	}
}

func TestMaterialUpdate_NoopKeepsSnapshot(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-noop")
	regID, _, _, _ := mustActiveRegistration(t, f, userID, "key-material-noop1")

	resp, err := f.svc.UpdateText(ctx, userID, regID, MaterialTextInput{})
	if err != nil {
		t.Fatalf("noop Update: %v", err)
	}
	if resp.MaterialStatus.Completed != 0 || len(resp.MaterialStatus.Missing) != 8 {
		t.Fatalf("material_status = %+v", resp.MaterialStatus)
	}
	if resp.Materials.UpdatedAt != nil {
		t.Fatalf("updated_at = %v, want nil", resp.Materials.UpdatedAt)
	}
}

// ---- 文件字段 ----

func TestMaterialUpdate_PhotoAndAvatar(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-photo")
	regID, eventID, playerID, playerNo := mustActiveRegistration(t, f, userID, "key-material-photo")

	photo := testMaterialFile("id_photo.jpg", testJPEG(t, 1000, 500))
	resp, err := f.svc.UpdatePhoto(ctx, userID, regID, photo)
	if err != nil {
		t.Fatalf("Update photo: %v", err)
	}
	if resp.Materials.PhotoURL == nil || !strings.HasPrefix(*resp.Materials.PhotoURL,
		"/static/materials/"+strconv.FormatInt(eventID, 10)+"/"+playerNo+"/photo_") {
		t.Fatalf("photo_url = %v", resp.Materials.PhotoURL)
	}

	var photoPath string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT photo_path FROM registrations WHERE id = ?`, regID).Scan(&photoPath); err != nil {
		t.Fatalf("query photo_path: %v", err)
	}
	if _, err := os.Stat(f.abs(photoPath)); err != nil {
		t.Fatalf("photo file missing: %v", err)
	}

	var avatarPath string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT avatar_path FROM players WHERE id = ?`, playerID).Scan(&avatarPath); err != nil {
		t.Fatalf("query avatar_path: %v", err)
	}
	if !strings.HasPrefix(avatarPath, "uploads/avatars/"+playerNo+"/avatar_") || !strings.HasSuffix(avatarPath, ".jpg") {
		t.Fatalf("avatar_path = %q", avatarPath)
	}
	avatarFile, err := os.Open(f.abs(avatarPath))
	if err != nil {
		t.Fatalf("open avatar: %v", err)
	}
	defer avatarFile.Close()
	cfg, format, err := image.DecodeConfig(avatarFile)
	if err != nil {
		t.Fatalf("decode avatar config: %v", err)
	}
	if format != "jpeg" || cfg.Width != 512 || cfg.Height != 256 {
		t.Fatalf("avatar = %s %dx%d, want jpeg 512x256", format, cfg.Width, cfg.Height)
	}

	var photoPresent int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM registrations WHERE id = ? AND photo_path IS NOT NULL`, regID).Scan(&photoPresent); err != nil {
		t.Fatalf("count photo: %v", err)
	}
	if photoPresent != 1 || resp.MaterialStatus.Completed != 1 {
		t.Fatalf("photo present = %d, completed = %d", photoPresent, resp.MaterialStatus.Completed)
	}
}

func TestMaterialUpdate_ReplacePhotoCleansOldFiles(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-replace")
	regID, _, playerID, _ := mustActiveRegistration(t, f, userID, "key-material-repl1")

	if _, err := f.svc.UpdatePhoto(ctx, userID, regID, testMaterialFile("a.jpg", testJPEG(t, 800, 600))); err != nil {
		t.Fatalf("first photo: %v", err)
	}
	var oldPhoto, oldAvatar string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT photo_path FROM registrations WHERE id = ?`, regID).Scan(&oldPhoto); err != nil {
		t.Fatalf("old photo: %v", err)
	}
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT avatar_path FROM players WHERE id = ?`, playerID).Scan(&oldAvatar); err != nil {
		t.Fatalf("old avatar: %v", err)
	}

	if _, err := f.svc.UpdatePhoto(ctx, userID, regID, testMaterialFile("b.png", pngBytes(t))); err != nil {
		t.Fatalf("second photo: %v", err)
	}
	var newPhoto, newAvatar string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT photo_path FROM registrations WHERE id = ?`, regID).Scan(&newPhoto); err != nil {
		t.Fatalf("new photo: %v", err)
	}
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT avatar_path FROM players WHERE id = ?`, playerID).Scan(&newAvatar); err != nil {
		t.Fatalf("new avatar: %v", err)
	}
	if newPhoto == oldPhoto || newAvatar == oldAvatar {
		t.Fatalf("paths not replaced: photo %q→%q avatar %q→%q", oldPhoto, newPhoto, oldAvatar, newAvatar)
	}
	if _, err := os.Stat(f.abs(oldPhoto)); !os.IsNotExist(err) {
		t.Fatalf("old photo still exists: %v", err)
	}
	if _, err := os.Stat(f.abs(oldAvatar)); !os.IsNotExist(err) {
		t.Fatalf("old avatar still exists: %v", err)
	}
	if _, err := os.Stat(f.abs(newPhoto)); err != nil {
		t.Fatalf("new photo missing: %v", err)
	}
	if _, err := os.Stat(f.abs(newAvatar)); err != nil {
		t.Fatalf("new avatar missing: %v", err)
	}
}

func TestMaterialUpdate_Audio(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-audio")
	regID, _, _, _ := mustActiveRegistration(t, f, userID, "key-material-audio")

	resp, err := f.svc.UpdateAudio(ctx, userID, regID, testMaterialFile("dance.mp3", testMP3(100)))
	if err != nil {
		t.Fatalf("Update audio: %v", err)
	}
	if resp.Materials.AudioURL == nil || !strings.HasSuffix(*resp.Materials.AudioURL, ".mp3") {
		t.Fatalf("audio_url = %v", resp.Materials.AudioURL)
	}
	if resp.Materials.AudioFilename == nil || !strings.HasPrefix(*resp.Materials.AudioFilename, "audio_") {
		t.Fatalf("audio_filename = %v", resp.Materials.AudioFilename)
	}

	var audioPath string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT audio_path FROM registrations WHERE id = ?`, regID).Scan(&audioPath); err != nil {
		t.Fatalf("query audio_path: %v", err)
	}
	if _, err := os.Stat(f.abs(audioPath)); err != nil {
		t.Fatalf("audio file missing: %v", err)
	}

	// 超过 4 分钟：9190 帧 ≈ 4.0 分钟 + 数毫秒。
	_, err = f.svc.UpdateAudio(ctx, userID, regID, testMaterialFile("long.mp3", testMP3(9190)))
	requireBizCode(t, err, httpx.CodeBadRequest)
	var after string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT audio_path FROM registrations WHERE id = ?`, regID).Scan(&after); err != nil {
		t.Fatalf("query audio after reject: %v", err)
	}
	if after != audioPath {
		t.Fatalf("audio_path changed after rejected upload: %q → %q", audioPath, after)
	}
}
