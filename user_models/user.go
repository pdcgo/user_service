package user_models

import (
	"time"

	"github.com/pdcgo/schema/services/role_base/v1"
)

type User struct {
	ID             uint   `json:"id" gorm:"primarykey"`
	Name           string `json:"name"`
	ProfilePicture string `json:"profile_picture"`
	Username       string `json:"username" gorm:"index:username_unique,unique"`
	Password       string `json:"-"`
	Email          string `json:"email" gorm:"index:email_unique,unique"`
	PhoneNumber    string `json:"phone_number"`
	IsSuspended    bool   `json:"is_suspend"`

	LastPasswordReset time.Time `json:"last_password_reset"`
	CreatedAt         time.Time `json:"created"`
}

type UserTeamRole struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	TeamID    uint           `json:"team_id" gorm:"index:team_user_unique,unique"`
	UserID    uint           `json:"user_id" gorm:"index:team_user_unique,unique"`
	Role      role_base.Role `json:"role"`
	Alias     string         `json:"alias"`
	CreatedAt time.Time      `json:"created"`
}
