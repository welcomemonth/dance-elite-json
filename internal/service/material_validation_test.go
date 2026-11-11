package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

func TestMaterialUpdate_RejectsInvalidFiles(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-invalid")
	regID, _, _, _ := mustActiveRegistration(t, f, userID, "key-material-inval")

	cases := []struct {
		name  string
		input MaterialUpdateInput
	}{
		{"photo wrong ext", MaterialUpdateInput{Photo: testMaterialFile("a.gif", testJPEG(t, 10, 10))}},
		{"photo content mismatch", MaterialUpdateInput{Photo: testMaterialFile("a.png", testJPEG(t, 10, 10))}},
		{"photo fake content", MaterialUpdateInput{Photo: testMaterialFile("a.jpg", []byte("not an image"))}},
		{"photo too large", MaterialUpdateInput{Photo: &MaterialFile{
			Filename: "a.jpg", Size: MaxPhotoBytes + 1, Content: readSeekCloser{bytes.NewReader(nil)},
		}}},
		{"audio wrong ext", MaterialUpdateInput{Audio: testMaterialFile("dance.wav", testMP3(10))}},
		{"audio invalid content", MaterialUpdateInput{Audio: testMaterialFile("dance.mp3", []byte("not mp3"))}},
		{"audio too large", MaterialUpdateInput{Audio: &MaterialFile{
			Filename: "dance.mp3", Size: MaxAudioBytes + 1, Content: readSeekCloser{bytes.NewReader(nil)},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.Update(ctx, userID, regID, tc.input)
			requireBizCode(t, err, httpx.CodeBadRequest)
		})
	}

	// 失败的校验不应产生任何资料字段。
	var photo, audio *string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT photo_path, audio_path FROM registrations WHERE id = ?`, regID).Scan(&photo, &audio); err != nil {
		t.Fatalf("query paths: %v", err)
	}
	if photo != nil || audio != nil {
		t.Fatalf("paths set after rejected uploads: photo=%v audio=%v", photo, audio)
	}
}

func TestMaterialUpdate_TextValidation(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-textval")
	regID, _, _, _ := mustActiveRegistration(t, f, userID, "key-material-textv")

	badPhone := "12345"
	_, err := f.svc.Update(ctx, userID, regID, MaterialUpdateInput{ContactPhone: &badPhone})
	requireBizCode(t, err, httpx.CodeBadRequest)

	longName := strings.Repeat("舞", 51)
	_, err = f.svc.Update(ctx, userID, regID, MaterialUpdateInput{ProgramName: &longName})
	requireBizCode(t, err, httpx.CodeBadRequest)

	if _, err := f.db.SQL().ExecContext(ctx,
		`INSERT INTO settings ("key", value) VALUES ('sensitive_words', '["违禁"]')`); err != nil {
		t.Fatalf("seed sensitive words: %v", err)
	}
	bad := "含违禁内容的节目"
	_, err = f.svc.Update(ctx, userID, regID, MaterialUpdateInput{ProgramName: &bad})
	requireBizCode(t, err, httpx.CodeBadRequest)

	// 长度按字符（rune）计：50 个汉字可以，51 个不行。
	ok := strings.Repeat("舞", 50)
	if _, err := f.svc.Update(ctx, userID, regID, MaterialUpdateInput{ProgramName: &ok}); err != nil {
		t.Fatalf("50-rune program name rejected: %v", err)
	}
}

func TestMaterialUpdate_OwnershipStatusAndDeadline(t *testing.T) {
	f := newMaterialFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-material-owner")
	otherID := mustInsertRegUser(t, f.db, "o-material-other")
	regID, eventID, _, _ := mustActiveRegistration(t, f, userID, "key-material-owner")

	value := "内容"
	_, err := f.svc.Update(ctx, otherID, regID, MaterialUpdateInput{ProgramName: &value})
	requireBizCode(t, err, httpx.CodeNotFound)
	_, err = f.svc.Update(ctx, userID, regID+99999, MaterialUpdateInput{ProgramName: &value})
	requireBizCode(t, err, httpx.CodeNotFound)

	for _, status := range []string{
		model.RegistrationStatusCanceled,
		model.RegistrationStatusWithdrawn,
		model.RegistrationStatusPendingPayment,
	} {
		if _, err := f.db.SQL().ExecContext(ctx,
			`UPDATE registrations SET status = ? WHERE id = ?`, status, regID); err != nil {
			t.Fatalf("set status %s: %v", status, err)
		}
		_, err := f.svc.Update(ctx, userID, regID, MaterialUpdateInput{ProgramName: &value})
		requireBizCode(t, err, httpx.CodeMaterialNotAllowed)
	}
	if _, err := f.db.SQL().ExecContext(ctx,
		`UPDATE registrations SET status = 'active' WHERE id = ?`, regID); err != nil {
		t.Fatalf("reactivate: %v", err)
	}

	// 资料截止时间设为过去（时间线整体前移以满足 CHECK 约束）。
	if _, err := f.db.SQL().ExecContext(ctx, `
		UPDATE events SET
			reg_start_at = '2020-01-01T00:00:00+08:00',
			reg_end_at = '2020-01-02T00:00:00+08:00',
			lottery_at = '2020-01-03T00:00:00+08:00',
			material_check_at = '2020-01-04T00:00:00+08:00',
			start_at = '2020-01-05T00:00:00+08:00'
		WHERE id = ?`, eventID); err != nil {
		t.Fatalf("move timeline to past: %v", err)
	}
	_, err = f.svc.Update(ctx, userID, regID, MaterialUpdateInput{ProgramName: &value})
	requireBizCode(t, err, httpx.CodeMaterialDeadline)
}
