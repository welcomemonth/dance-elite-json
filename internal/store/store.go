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

func (s *Store) GetRankGroup(ctx context.Context) (*model.RankGroup, error) {
	path := filepath.Join(s.dsn, "ranking.jsonc")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rankGroup model.RankGroup
	if err := json.NewDecoder(f).Decode(&rankGroup); err != nil {
		return nil, err
	}
	return &rankGroup, nil
}
