package store

import (
	"encoding/json"
	"os"

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
	f, err := os.Open(s.dsn + "config.json")
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
