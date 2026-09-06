package service

import (
	"github.com/meetmonth/dance-elite/internal/model"
)

// Store 是 service 层所需数据访问的最小接口。
//
// 接口定义在「消费方」（service），由生产方 store.Store 隐式实现，
// 测试时用一个 fake 实现替换即可，无需触碰真实文件系统。
type Store interface {
	GetConfig() (*model.Config, error)
}

type APIService struct {
	store Store
}

func NewService(store Store) *APIService {
	return &APIService{
		store: store,
	}
}

func (s *APIService) GetConfig() (*model.Config, error) {
	return s.store.GetConfig()
}
