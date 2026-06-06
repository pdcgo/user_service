package user

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
	"gorm.io/gorm/clause"
)

// TeamUserUpdate implements [user_ifaceconnect.V2UserServiceHandler]. The oneof
// action either adds a user to the team (upserting their role) or removes them.
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
			Role:   uint(add.Role),
		}
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "team_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"role"}),
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

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("add or remove is required"))
	}

	return connect.NewResponse(&user_iface.TeamUserUpdateResponse{}), nil
}
