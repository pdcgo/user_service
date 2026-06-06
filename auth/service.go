package auth

import (
	"github.com/pdcgo/san_collection/san_caches"
	"gorm.io/gorm"
)

type v2AuthServiceImpl struct {
	db       *gorm.DB
	secret   string
	cacheMgr san_caches.CacheManager
}

func NewV2AuthService(
	db *gorm.DB,
	secret string,
	cacheMgr san_caches.CacheManager,
) *v2AuthServiceImpl {
	return &v2AuthServiceImpl{
		db:       db,
		secret:   secret,
		cacheMgr: cacheMgr,
	}
}
