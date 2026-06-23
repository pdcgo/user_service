package user_service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/pdcgo/san_collection/san_execution"
	"github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/user_service/user_models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserServiceSeeder func(ctx context.Context, db *gorm.DB) error

type Team struct {
	ID   uint64 `gorm:"primarykey"`
	Name string
	Type string
}

type User struct {
	ID        uint64 `gorm:"primarykey"`
	Name      string
	Email     string
	Username  string
	Password  string
	CreatedAt time.Time
}

func NewUserServiceSeeder() UserServiceSeeder {
	return func(ctx context.Context, db *gorm.DB) error {
		var err error

		slog.Info("running user_service seeder")

		slog.Info("getting root team")
		var team Team

		err = db.
			Table("teams t").
			Where("t.id = ?", 1).
			Find(&team).
			Error

		if err != nil {
			return err
		}

		if team.ID == 0 {
			slog.Info("creating root team, root team not exist")
			team = Team{
				ID:   1,
				Name: "Root Team",
				Type: "root",
			}

			err = db.Save(&team).Error
			if err != nil {
				return err
			}
		}

		slog.Info("check user root")
		var user User

		err = db.
			Table("users u").
			Where("u.id = ?", 1).
			Find(&user).
			Error

		if err != nil {
			return err
		}

		if user.ID == 0 {
			slog.Info("creating root user, root user not exist")
			user = User{
				ID:        1,
				Username:  "root",
				Password:  seedPassword("test123"),
				Name:      "Root",
				Email:     "root@system.com",
				CreatedAt: time.Now(),
			}

			err = db.Save(&user).Error
			if err != nil {
				return err
			}
		}

		userRole := user_models.UserTeamRole{
			TeamID:    uint(team.ID),
			UserID:    uint(user.ID),
			Role:      role_base.Role_ROLE_ROOT,
			Alias:     "root",
			CreatedAt: time.Now(),
		}

		err = db.Save(&userRole).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrDuplicatedKey) {
				return err
			}
		}

		handler := san_execution.NewChainParam(
			seedSellingTeam,
			seedUserSapuJagad,
		)

		_, err = handler(db)
		return err
	}
}

func seedUserSapuJagad(next san_execution.NextFuncParam[*gorm.DB]) san_execution.NextFuncParam[*gorm.DB] {
	return func(db *gorm.DB) (*gorm.DB, error) {
		var err error

		slog.Info("check user sapujagad")
		var user User

		err = db.
			Table("users u").
			Where("u.username = ?", "sapujagad").
			Find(&user).
			Error

		if err != nil {
			return nil, err
		}

		if user.ID == 0 {
			slog.Info("creating sapujagad user, sapujagad user not exist")
			user = User{
				Username:  "sapujagad",
				Password:  seedPassword("test123"),
				Name:      "Sapu Jagad",
				Email:     "sapujagad@system.com",
				CreatedAt: time.Now(),
			}

			err = db.Save(&user).Error
			if err != nil {
				return nil, err
			}
		}

		teamIDs := []uint64{}
		err = db.
			Table("teams t").
			Select("t.id").
			Find(&teamIDs).
			Error

		if err != nil {
			return nil, err
		}

		for _, teamId := range teamIDs {
			userRole := user_models.UserTeamRole{
				TeamID:    uint(teamId),
				UserID:    uint(user.ID),
				Role:      role_base.Role_ROLE_TEAM_OWNER,
				Alias:     "own",
				CreatedAt: time.Now(),
			}

			err = db.Save(&userRole).Error
			if err != nil {
				if !errors.Is(err, gorm.ErrDuplicatedKey) {
					return nil, err
				}
			}
		}

		return next(db)
	}
}

func seedSellingTeam(next san_execution.NextFuncParam[*gorm.DB]) san_execution.NextFuncParam[*gorm.DB] {
	return func(db *gorm.DB) (*gorm.DB, error) {

		var err error

		slog.Info("getting selling team")
		var team Team

		err = db.
			Table("teams t").
			Where("t.type = ?", "selling").
			Find(&team).
			Error

		if err != nil {
			return nil, err
		}

		if team.ID == 0 {
			slog.Info("creating selling team, selling team not exist")
			team = Team{
				Name: "Selling Team",
				Type: "selling",
			}

			err = db.Save(&team).Error
			if err != nil {
				return nil, err
			}
		}

		slog.Info("check user selling")
		var userId uint64

		err = db.
			Table("user_team_roles r").
			Where("r.team_id = ?", team.ID).
			Where("r.role = ?", role_base.Role_ROLE_TEAM_OWNER).
			Select("r.user_id").
			Find(&userId).
			Error

		if err != nil {
			return nil, err
		}

		if userId == 0 {
			slog.Info("creating selling user, selling user not exist")
			user := User{
				Username:  "selling_01",
				Password:  seedPassword("test123"),
				Name:      "Selling 01",
				Email:     "selling01@system.com",
				CreatedAt: time.Now(),
			}

			err = db.Save(&user).Error
			if err != nil {
				return nil, err
			}

			userRole := user_models.UserTeamRole{
				TeamID:    uint(team.ID),
				UserID:    uint(user.ID),
				Role:      role_base.Role_ROLE_TEAM_OWNER,
				Alias:     "own",
				CreatedAt: time.Now(),
			}

			err = db.Save(&userRole).Error
			if err != nil {
				if !errors.Is(err, gorm.ErrDuplicatedKey) {
					return nil, err
				}
			}
		}

		return next(db)

	}
}

func seedPassword(pass string) string {
	encPassword, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	return string(encPassword)
}
