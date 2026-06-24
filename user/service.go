package user

import (
	"github.com/pdcgo/san_collection/san_verification"
	"gorm.io/gorm"
)

type v2UserServiceImpl struct {
	db  *gorm.DB
	otp san_verification.OtpVerification
}

func NewV2UserService(db *gorm.DB, otp san_verification.OtpVerification) *v2UserServiceImpl {
	return &v2UserServiceImpl{db: db, otp: otp}
}
