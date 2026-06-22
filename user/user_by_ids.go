package user

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
)

// UserByIDs implements [user_ifaceconnect.V2UserServiceHandler]. It returns the
// requested users keyed by id (missing ids are omitted). When team_id is set,
// each user that is a member of that team also carries its per-team UserAlias
// (alias + role); non-members, and a zero team_id, leave alias unset.
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

	// per-team alias + role, only when a team is requested.
	memberByUser := map[uint]user_models.UserTeamRole{}
	if pay.TeamId != 0 {
		var roles []user_models.UserTeamRole
		if err := db.
			Where("team_id = ? AND user_id IN ?", pay.TeamId, pay.Ids).
			Find(&roles).Error; err != nil {
			return nil, err
		}
		for _, r := range roles {
			memberByUser[r.UserID] = r
		}
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
		if r, ok := memberByUser[u.ID]; ok {
			item.Alias = &user_iface.UserAlias{
				TeamId: pay.TeamId,
				Alias:  r.Alias,
				Role:   r.Role,
			}
		}
		resp.Users[uint64(u.ID)] = item
	}

	return connect.NewResponse(resp), nil
}
