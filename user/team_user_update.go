package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TeamUserUpdate implements [user_ifaceconnect.V2UserServiceHandler]. The oneof
// action adds an existing user to the team (upserting their role+alias), creates a
// new user and adds them to the team, or removes a member.
func (s *v2UserServiceImpl) TeamUserUpdate(
	ctx context.Context,
	req *connect.Request[user_iface.TeamUserUpdateRequest],
) (*connect.Response[user_iface.TeamUserUpdateResponse], error) {
	db := s.db.WithContext(ctx)
	teamID := req.Msg.TeamId

	switch {
	case req.Msg.GetAdd() != nil:
		add := req.Msg.GetAdd()
		rec := &user_models.UserTeamRole{
			TeamID: uint(teamID),
			UserID: uint(add.UserId),
			Role:   add.Role,
			Alias:  add.Alias,
		}
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "team_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"role", "alias"}),
		}).Create(rec).Error
		if err != nil {
			return nil, err
		}

	case req.Msg.GetRemove() != nil:
		rem := req.Msg.GetRemove()
		err := db.
			Where("team_id = ? AND user_id = ?", teamID, rem.UserId).
			Delete(&user_models.UserTeamRole{}).
			Error
		if err != nil {
			return nil, err
		}

	case req.Msg.GetCreate() != nil:
		c := req.Msg.GetCreate()
		hash, err := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		now := time.Now()
		usr := &user_models.User{
			Name:              c.Name,
			Username:          strings.ToLower(strings.TrimSpace(c.Username)),
			Email:             strings.ToLower(strings.TrimSpace(c.Email)),
			Password:          string(hash),
			PhoneNumber:       strings.TrimSpace(c.PhoneNumber),
			CreatedAt:         now,
			LastPasswordReset: now,
		}
		// Create the user and attach the team role atomically.
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(usr).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					return connect.NewError(connect.CodeAlreadyExists, errors.New("username or email already exists"))
				}
				return err
			}
			return tx.Create(&user_models.UserTeamRole{
				TeamID: uint(teamID),
				UserID: usr.ID,
				Role:   c.Role,
				Alias:  c.Alias,
			}).Error
		})
		if err != nil {
			return nil, err
		}

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("add, remove, or create is required"))
	}

	return connect.NewResponse(&user_iface.TeamUserUpdateResponse{}), nil
}
