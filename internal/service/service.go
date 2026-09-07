package service

import (
	"context"

	"github.com/meetmonth/dance-elite/internal/model"
)

// Store 是 service 层所需数据访问的最小接口。
//
// 接口定义在「消费方」（service），由生产方 store.Store 隐式实现，
// 测试时用一个 fake 实现替换即可，无需触碰真实文件系统。
type Store interface {
	GetConfig(ctx context.Context) (*model.Config, error)
	GetRankGroups(ctx context.Context) ([]*model.RankGroup, error)
	GetDirectDancers(ctx context.Context) ([]*model.Dancer, error)
}

type APIService struct {
	store Store
}

func NewService(store Store) *APIService {
	return &APIService{
		store: store,
	}
}

func (s *APIService) GetConfig(ctx context.Context) (*model.Config, error) {
	return s.store.GetConfig(ctx)
}

func (s *APIService) GetRankGroups(ctx context.Context) ([]*model.RankGroup, error) {
	return s.store.GetRankGroups(ctx)
}

func (s *APIService) GetDirectDancers(ctx context.Context) ([]*model.Dancer, error) {
	dancers, err := s.store.GetDirectDancers(ctx)

	var directDancers []*model.Dancer
	if err != nil {
		return directDancers, err
	}

	for _, dancer := range dancers {
		if dancer.IsDirect {
			directDancers = append(directDancers, dancer)
		}
	}
	return directDancers, err
}
