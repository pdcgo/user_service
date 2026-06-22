package user

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
)

// UserByIDs implements [user_ifaceconnect.V2UserServiceHandler]. It returns the
// requested users keyed by id (missing ids are omitted). When team_id is set,
// each member of that team carries a single-element per-team UserAlias list
// (alias + role) and non-members carry none. When team_id is 0, every user
// carries one UserAlias per team they belong to (all teams).
func (s *v2UserServiceImpl) UserByIDs(
	ctx context.Context,
	req *connect.Request[user_iface.UserByIDsRequest],
) (*connect.Response[user_iface.UserByIDsResponse], error) {
	pay := req.Msg
	resp := &user_iface.UserByIDsResponse{Users: map[uint64]*user_iface.UserMapItem{}}
	if len(pay.Ids) == 0 {
		return connect.NewResponse(resp), nil
	}

	db := s.db.WithContext(ctx)

	var users []user_models.User
	if err := db.Where("id IN ?", pay.Ids).Find(&users).Error; err != nil {
		return nil, err
	}

	// per-team alias + role. team_id set -> just that team (<=1 entry per user);
	// team_id == 0 -> every team each user belongs to.
	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	aliases, err := loadTeamAliases(db, ids, pay.TeamId)
	if err != nil {
		return nil, err
	}

	for _, u := range users {
		status := user_iface.UserStatus_USER_STATUS_ACTIVE
		if u.IsSuspended {
			status = user_iface.UserStatus_USER_STATUS_SUSPENDED
		}

		item := &user_iface.UserMapItem{
			User: &user_iface.UserInfo{
				Id:             uint64(u.ID),
				Email:          u.Email,
				Username:       u.Username,
				PhoneNumber:    u.PhoneNumber,
				Name:           u.Name,
				Status:         status,
				ProfilePicture: u.ProfilePicture,
			},
		}
		item.Alias = aliases[u.ID]
		resp.Users[uint64(u.ID)] = item
	}

	return connect.NewResponse(resp), nil
}
