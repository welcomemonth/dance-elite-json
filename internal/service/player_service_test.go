package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 与 utils/idcard_test.go 手工核算向量一致的合法身份证。
const (
	validIDCardMale2016   = "110105201603070010" // 2016-03-07 男
	validIDCardFemale2015 = "110105201501020020" // 2015-01-02 女
	validIDCardMale2017   = "110105201701020033" // 2017-01-02 男（并发/取消测试的第二名选手）
)

func newPlayerFixture(t *testing.T) (*PlayerService, *store.Store) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stores := store.New(db)
	return NewPlayerService(stores, "test-hmac-key"), stores
}

func mustPrepare(t *testing.T, svc *PlayerService, in IdentityInput) *PreparedIdentity {
	t.Helper()
	prepared, err := svc.PrepareIdentity(in)
	if err != nil {
		t.Fatalf("PrepareIdentity(%+v): %v", in, err)
	}
	return prepared
}

func TestPrepareIdentity_IDCardParsesBirthAndGender(t *testing.T) {
	svc, _ := newPlayerFixture(t)

	p := mustPrepare(t, svc, IdentityInput{IDNumber: validIDCardMale2016, Name: " 王小舞 "})
	if p.IDType != utils.IDTypeIDCard || p.Name != "王小舞" {
		t.Errorf("unexpected prepared: %+v", p)
	}
	if p.BirthDate == nil || *p.BirthDate != "2016-03-07" {
		t.Errorf("birth_date = %v, want 2016-03-07", p.BirthDate)
	}
	if p.Gender == nil || *p.Gender != model.GenderMale {
		t.Errorf("gender = %v, want male", p.Gender)
	}
	wantHash := utils.HashIDNumber([]byte("test-hmac-key"), utils.IDTypeIDCard, validIDCardMale2016)
	if p.IDCardHash != wantHash {
		t.Errorf("hash mismatch with utils.HashIDNumber")
	}

	// 女 + 含空格输入：规范化后与上面同值。
	q := mustPrepare(t, svc, IdentityInput{IDNumber: " 110105201501020020 ", Name: "李小菁"})
	if *q.Gender != model.GenderFemale || *q.BirthDate != "2015-01-02" {
		t.Errorf("unexpected female parse: %+v", q)
	}
}

func TestPrepareIdentity_InvalidIDCard(t *testing.T) {
	svc, _ := newPlayerFixture(t)

	for _, raw := range []string{"123", "110105201603070012", "11010520160307001x "} {
		_, err := svc.PrepareIdentity(IdentityInput{IDNumber: raw, Name: "王小舞"})
		var biz *httpx.BizError
		if !errors.As(err, &biz) || biz.Code != httpx.CodeInvalidIDNumber {
			t.Errorf("PrepareIdentity(%q) err = %v, want 41001", raw, err)
		}
	}
}

func TestPrepareIdentity_Passport(t *testing.T) {
	svc, _ := newPlayerFixture(t)

	// 护照：号码作身份键，出生日期必须人工提供；性别可选。
	p, err := svc.PrepareIdentity(IdentityInput{
		IDType: utils.IDTypePassport, IDNumber: "e12345678 ", Name: "Anna Li", BirthDate: "2014-05-20", Gender: "female",
	})
	if err != nil {
		t.Fatalf("PrepareIdentity passport: %v", err)
	}
	if *p.BirthDate != "2014-05-20" || *p.Gender != model.GenderFemale {
		t.Errorf("unexpected passport prepared: %+v", p)
	}
	wantHash := utils.HashIDNumber([]byte("test-hmac-key"), utils.IDTypePassport, "E12345678")
	if p.IDCardHash != wantHash {
		t.Errorf("passport hash mismatch (normalization should uppercase and strip spaces)")
	}

	// 缺出生日期 → 40001。
	if _, err := svc.PrepareIdentity(IdentityInput{
		IDType: utils.IDTypePassport, IDNumber: "E12345678", Name: "Anna Li",
	}); err == nil {
		t.Fatal("expected error for missing birth_date")
	} else {
		var biz *httpx.BizError
		if !errors.As(err, &biz) || biz.Code != httpx.CodeBadRequest {
			t.Errorf("err = %v, want 40001", err)
		}
	}
}

func TestMatchOrCreate_IDCardTwiceSamePlayer(t *testing.T) {
	svc, stores := newPlayerFixture(t)

	var firstID, secondID int64
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		p, created, err := svc.MatchOrCreate(context.Background(), tx,
			IdentityInput{IDNumber: validIDCardMale2016, Name: "王小舞"})
		if err != nil {
			return err
		}
		if !created {
			t.Fatal("first match should create")
		}
		firstID = p.ID
		return nil
	})
	if err != nil {
		t.Fatalf("tx1: %v", err)
	}

	err = stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		// 不同输入形式（首尾空格）→ 同一档案。
		p, created, err := svc.MatchOrCreate(context.Background(), tx,
			IdentityInput{IDNumber: " 110105201603070010 ", Name: "王小舞"})
		if err != nil {
			return err
		}
		if created {
			t.Fatal("second match should not create")
		}
		secondID = p.ID
		return nil
	})
	if err != nil {
		t.Fatalf("tx2: %v", err)
	}
	if firstID != secondID {
		t.Errorf("same id number matched different players: %d vs %d", firstID, secondID)
	}
}

func TestBirthYear_AndAgeGroupMatching(t *testing.T) {
	groups := []model.EventGroup{
		{ID: 1, Level: model.GroupLevelU11, BirthYearFrom: 2016, BirthYearTo: 2018},
		{ID: 2, Level: model.GroupLevelU13, BirthYearFrom: 2014, BirthYearTo: 2015},
		{ID: 3, Level: model.GroupLevelU15, BirthYearFrom: 2012, BirthYearTo: 2013},
	}

	// 闭区间边界：2016 → U11，2015 → U13，2012 → U15。
	cases := map[int]string{
		2018: model.GroupLevelU11,
		2016: model.GroupLevelU11,
		2015: model.GroupLevelU13,
		2014: model.GroupLevelU13,
		2013: model.GroupLevelU15,
		2012: model.GroupLevelU15,
	}
	for year, wantLevel := range cases {
		matched := MatchGroupsByBirthYear(year, groups)
		if len(matched) != 1 || matched[0].Level != wantLevel {
			t.Errorf("year %d matched %+v, want only %s", year, matched, wantLevel)
		}
	}

	// 超龄（2010）与未达（2020）均无匹配。
	for _, year := range []int{2010, 2020} {
		if matched := MatchGroupsByBirthYear(year, groups); len(matched) != 0 {
			t.Errorf("year %d matched %+v, want none", year, matched)
		}
	}
}

func TestCheckAgeFit_ErrorCarriesRanges(t *testing.T) {
	groups := []model.EventGroup{
		{Level: model.GroupLevelU11, BirthYearFrom: 2016, BirthYearTo: 2018},
		{Level: model.GroupLevelU13, BirthYearFrom: 2014, BirthYearTo: 2015},
	}

	// 命中：返回匹配组别，无错误。
	matched, err := CheckAgeFit(2016, groups)
	if err != nil || len(matched) != 1 || matched[0].Level != model.GroupLevelU11 {
		t.Fatalf("CheckAgeFit(2016) = %+v, %v", matched, err)
	}

	// 不匹配：41002，消息附组别出生年份区间。
	_, err = CheckAgeFit(2010, groups)
	var biz *httpx.BizError
	if !errors.As(err, &biz) || biz.Code != httpx.CodeAgeNotAllowed {
		t.Fatalf("err = %v, want 41002", err)
	}
	for _, want := range []string{"2010", "U11", "2016-2018", "U13", "2014-2015"} {
		if !strings.Contains(biz.Message, want) {
			t.Errorf("message %q should contain %q", biz.Message, want)
		}
	}
}
