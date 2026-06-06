package user

import "gorm.io/gorm"

type v2UserServiceImpl struct {
	db *gorm.DB
}

func NewV2UserService(db *gorm.DB) *v2UserServiceImpl {
	return &v2UserServiceImpl{db: db}
}
