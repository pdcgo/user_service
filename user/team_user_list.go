package user

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
)

type teamMember struct {
	ID             uint
	Name           string
	Email          string
	Username       string
	PhoneNumber    string
	IsSuspended    bool
	Role           uint
	ProfilePicture string
}

// TeamUserList implements [user_ifaceconnect.V2UserServiceHandler]. It returns
// the members of the given team (with their role), optionally filtered by role
// and a name/email/username search.
func (s *v2UserServiceImpl) TeamUserList(
	ctx context.Context,
	req *connect.Request[user_iface.TeamUserListRequest],
) (*connect.Response[user_iface.TeamUserListResponse], error) {
	db := s.db.WithContext(ctx)

	q := db.
		Model(&user_models.UserTeamRole{}).
		Select("users.id, users.name, users.email, users.username, users.phone_number, users.is_suspended, user_team_roles.role, users.profile_picture").
		Joins("JOIN users ON users.id = user_team_roles.user_id").
		Where("user_team_roles.team_id = ?", req.Msg.TeamId)

	if req.Msg.Role != role_base.Role_ROLE_UNSPECIFIED {
		q = q.Where("user_team_roles.role = ?", uint(req.Msg.Role))
	}
	if s := strings.TrimSpace(req.Msg.Q); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		q = q.Where(
			"(lower(users.name) LIKE ? OR lower(users.email) LIKE ? OR lower(users.username) LIKE ?)",
			like, like, like,
		)
	}

	var rows []teamMember
	if err := q.Order("users.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	resp := &user_iface.TeamUserListResponse{Users: []*user_iface.User{}}
	for _, m := range rows {
		status := user_iface.UserStatus_USER_STATUS_ACTIVE
		if m.IsSuspended {
			status = user_iface.UserStatus_USER_STATUS_SUSPENDED
		}
		resp.Users = append(resp.Users, &user_iface.User{
			Id:             uint64(m.ID),
			Email:          m.Email,
			Username:       m.Username,
			PhoneNumber:    m.PhoneNumber,
			Name:           m.Name,
			Status:         status,
			Role:           role_base.Role(m.Role),
			ProfilePicture: m.ProfilePicture,
		})
	}

	return connect.NewResponse(resp), nil
}
