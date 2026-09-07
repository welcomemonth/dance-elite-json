package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/meetmonth/dance-elite/internal/model"
)

type Store struct {
	dsn string
}

func NewStore(dsn string) *Store {
	return &Store{
		dsn: dsn,
	}
}

func (s *Store) GetConfig(ctx context.Context) (*model.Config, error) {
	path := filepath.Join(s.dsn, "config.json")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var config model.Config
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Store) GetRankingDatas(ctx context.Context) (*model.RankingData, error) {
	path := filepath.Join(s.dsn, "ranking.json")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var RankingData model.RankingData
	if err := json.NewDecoder(f).Decode(&RankingData); err != nil {
		return nil, err
	}
	return &RankingData, nil
}

func (s *Store) GetDirectDancers(ctx context.Context) ([]*model.Dancer, error) {
	path := filepath.Join(s.dsn, "direct.jsonc")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var dancers []*model.Dancer
	if err := json.NewDecoder(f).Decode(&dancers); err != nil {
		return nil, err
	}
	return dancers, nil
}
