package service

import (
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/store"
)

type APIService struct {
	store *store.Store
}

func NewService(store *store.Store) *APIService {
	return &APIService{
		store: store,
	}
}

func (s *APIService) GetConfig() (*model.Config, error) {
	return s.store.GetConfig()
}
