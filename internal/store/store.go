package store

import (
	"context"
	"encoding/json"
	"fmt"
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

func (s *Store) GetInstitutionRank(ctx context.Context) (*model.InstitutionRanking, error) {
	path := filepath.Join(s.dsn, "institution.json")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var institutionRank model.InstitutionRanking
	err = json.NewDecoder(f).Decode(&institutionRank)
	if err != nil {
		return nil, err
	}
	return &institutionRank, nil
}

func (s *Store) GetDancerById(ctx context.Context, id string) (*model.Dancer, error) {
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

	var dancer model.Dancer
	for _, rank := range RankingData.Boards {
		for _, direct := range rank.Direct {
			if direct.ID == id {
				dancer.Avatar = direct.Avatar
				dancer.Awards = direct.Awards
				dancer.Name = direct.Name
				break
			}
		}
		for _, point := range rank.Ranking {
			if point.ID == id {
				dancer.Avatar = point.Avatar
				dancer.Awards = point.Awards
				dancer.Name = point.Name
				break
			}
		}
	}
	if dancer.Name == "" {
		return nil, fmt.Errorf("Can't find dancer by %s", id)
	}
	return &dancer, nil
}
