package store

import (
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

func (s *Store) GetConfig() (*model.Config, error) {
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
