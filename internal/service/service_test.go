package service

import (
	"context"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// fakeStore 实现 Store 接口，用于隔离真实文件系统。
type fakeStore struct {
	cfg             *model.Config
	RankingData     *model.RankingData
	directDancers   []*model.Dancer
	institutionRank *model.InstitutionRanking
	err             error
}

func (f fakeStore) GetConfig(ctx context.Context) (*model.Config, error) { return f.cfg, f.err }
func (f fakeStore) GetRankingDatas(ctx context.Context) (*model.RankingData, error) {
	return f.RankingData, f.err
}
func (f fakeStore) GetDirectDancers(ctx context.Context) ([]*model.Dancer, error) {
	return f.directDancers, f.err
}
func (f fakeStore) GetInstitutionRank(ctx context.Context) (*model.InstitutionRanking, error) {
	return f.institutionRank, f.err
}

func TestGetConfig(t *testing.T) {
	ctx := context.Background()
	want := &model.Config{Season: "2026"}
	svc := NewService(fakeStore{cfg: want})

	got, err := svc.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig 失败: %v", err)
	}
	if got.Season != "2026" {
		t.Errorf("Season = %q, want 2026", got.Season)
	}
}

func TestGetConfig_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("boom")
	svc := NewService(fakeStore{err: wantErr})

	if _, err := svc.GetConfig(ctx); !errors.Is(err, wantErr) {
		t.Errorf("期望透传 %v, got %v", wantErr, err)
	}
}
