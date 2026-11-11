package store

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/utils"
)

func newPlayerTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

var playerNoPattern = regexp.MustCompile(`^[A-Za-z0-9]{8}$`)

func mustMatchOrCreate(t *testing.T, s *Store, p PlayerParams) (*model.Player, bool) {
	t.Helper()
	var player *model.Player
	var created bool
	err := s.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		player, created, err = s.Players.MatchOrCreate(context.Background(), tx, p)
		return err
	})
	if err != nil {
		t.Fatalf("MatchOrCreate: %v", err)
	}
	return player, created
}

func TestPlayerMatchOrCreate_New(t *testing.T) {
	s := newPlayerTestStore(t)
	hash := utils.HashIDNumber([]byte("k"), utils.IDTypeIDCard, "110105201603070010")
	birth := "2016-03-07"
	gender := model.GenderMale

	player, created := mustMatchOrCreate(t, s, PlayerParams{
		Name: "王小舞", IDType: utils.IDTypeIDCard, IDCardHash: hash, BirthDate: &birth, Gender: &gender,
	})
	if !created {
		t.Fatal("created = false, want true")
	}
	if !playerNoPattern.MatchString(player.PlayerNo) {
		t.Errorf("player_no %q does not match 8-char alnum", player.PlayerNo)
	}
	if player.Name != "王小舞" || player.IDType != utils.IDTypeIDCard {
		t.Errorf("unexpected player: %+v", player)
	}
	if player.BirthDate == nil || *player.BirthDate != birth {
		t.Errorf("birth_date = %v, want %q", player.BirthDate, birth)
	}
	if player.Gender == nil || *player.Gender != gender {
		t.Errorf("gender = %v, want %q", player.Gender, gender)
	}
}

func TestPlayerMatchOrCreate_SameHashHitsSamePlayer(t *testing.T) {
	s := newPlayerTestStore(t)
	key := []byte("k")
	// 跨用户/跨输入形式（首尾空格）同证件号 → 同一档案（R4 规范化）。
	h1 := utils.HashIDNumber(key, utils.IDTypeIDCard, "110105201603070010")
	h2 := utils.HashIDNumber(key, utils.IDTypeIDCard, " 110105201603070010 ")

	p1, c1 := mustMatchOrCreate(t, s, PlayerParams{Name: "王小舞", IDType: utils.IDTypeIDCard, IDCardHash: h1})
	p2, c2 := mustMatchOrCreate(t, s, PlayerParams{Name: "王小舞", IDType: utils.IDTypeIDCard, IDCardHash: h2})
	if c1 != true || c2 != false {
		t.Fatalf("created = (%v, %v), want (true, false)", c1, c2)
	}
	if p1.ID != p2.ID || p1.PlayerNo != p2.PlayerNo {
		t.Errorf("same hash matched different players: %+v vs %+v", p1, p2)
	}
}

func TestPlayerMatchOrCreate_DifferentHashNewPlayer(t *testing.T) {
	s := newPlayerTestStore(t)
	key := []byte("k")
	h1 := utils.HashIDNumber(key, utils.IDTypeIDCard, "110105201603070010")
	h2 := utils.HashIDNumber(key, utils.IDTypeIDCard, "110105201501020020")

	p1, _ := mustMatchOrCreate(t, s, PlayerParams{Name: "王小舞", IDType: utils.IDTypeIDCard, IDCardHash: h1})
	p2, c2 := mustMatchOrCreate(t, s, PlayerParams{Name: "李小菁", IDType: utils.IDTypeIDCard, IDCardHash: h2})
	if !c2 || p1.ID == p2.ID {
		t.Errorf("different hash should create new player: %+v vs %+v", p1, p2)
	}
}

func TestPlayerMatchOrCreate_Passport(t *testing.T) {
	s := newPlayerTestStore(t)
	hash := utils.HashIDNumber([]byte("k"), utils.IDTypePassport, "E12345678")
	birth := "2014-05-20"

	player, created := mustMatchOrCreate(t, s, PlayerParams{
		Name: "Anna Li", IDType: utils.IDTypePassport, IDCardHash: hash, BirthDate: &birth,
	})
	if !created || player.IDType != utils.IDTypePassport {
		t.Fatalf("unexpected passport player: %+v created=%v", player, created)
	}
	if player.Gender != nil {
		t.Errorf("passport gender should stay nil, got %v", *player.Gender)
	}
}

func TestPlayerGetByHash_NotFound(t *testing.T) {
	s := newPlayerTestStore(t)
	_, err := s.Players.GetByHash(context.Background(), "no-such-hash")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
