package user

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/db_connect"
	"github.com/pdcgo/user_service/user_models"
	"gorm.io/gorm"
)

// UserList implements [user_ifaceconnect.V2UserServiceHandler]. With a team_id
// set it returns the members of that team, each as a UserMapItem carrying their
// single team-scoped UserAlias (alias + role). With team_id == 0 it returns all
// users (teamless users included, with an empty alias list), each carrying a
// UserAlias for every team they belong to — a privileged "all teams" view gated
// to root/admin callers by the access interceptor. Optionally filtered by role
// (membership role for a team; "holds the role in any team" when team_id == 0)
// and a name/email/username search.
func (s *v2UserServiceImpl) UserList(
	ctx context.Context,
	req *connect.Request[user_iface.UserListRequest],
) (*connect.Response[user_iface.UserListResponse], error) {
	if req.Msg.Page == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("page is required"))
	}

	db := s.db.WithContext(ctx)
	teamID := req.Msg.TeamId

	// Resolve the set of users to return. Select("users.*") keeps the count
	// subquery to the user columns (the team_id != 0 JOIN would otherwise carry
	// duplicate column names).
	var users []user_models.User
	paginated, pageInfo, err := db_connect.SetPaginationQuery(db, func() (*gorm.DB, error) {
		return db.
			Model(&user_models.User{}).
			Select("users.*").
			Scopes(func(d *gorm.DB) *gorm.DB {
				if teamID != 0 {
					// members of the given team, optionally filtered by their role there.
					d = d.Joins("JOIN user_team_roles utr ON utr.user_id = users.id AND utr.team_id = ?", teamID)
					if req.Msg.Role != role_base.Role_ROLE_UNSPECIFIED {
						d = d.Where("utr.role = ?", uint(req.Msg.Role))
					}
				} else if req.Msg.Role != role_base.Role_ROLE_UNSPECIFIED {
					// all users that hold the role in any team.
					d = d.Where(
						"EXISTS (SELECT 1 FROM user_team_roles utr WHERE utr.user_id = users.id AND utr.role = ?)",
						uint(req.Msg.Role),
					)
				}
				if s := strings.TrimSpace(req.Msg.Q); s != "" {
					like := "%" + strings.ToLower(s) + "%"
					d = d.Where(
						"(lower(users.name) LIKE ? OR lower(users.email) LIKE ? OR lower(users.username) LIKE ?)",
						like, like, like,
					)
				}
				return d
			}), nil
	}, req.Msg.Page)
	if err != nil {
		return nil, err
	}
	if err := paginated.Order("users.id ASC").Find(&users).Error; err != nil {
		return nil, err
	}

	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	aliases, err := loadTeamAliases(db, ids, teamID)
	if err != nil {
		return nil, err
	}

	resp := &user_iface.UserListResponse{
		Users:    make([]*user_iface.UserMapItem, 0, len(users)),
		PageInfo: pageInfo,
	}
	for _, u := range users {
		status := user_iface.UserStatus_USER_STATUS_ACTIVE
		if u.IsSuspended {
			status = user_iface.UserStatus_USER_STATUS_SUSPENDED
		}
		resp.Users = append(resp.Users, &user_iface.UserMapItem{
			User: &user_iface.UserInfo{
				Id:             uint64(u.ID),
				Email:          u.Email,
				Username:       u.Username,
				PhoneNumber:    u.PhoneNumber,
				Name:           u.Name,
				Status:         status,
				ProfilePicture: u.ProfilePicture,
			},
			Alias: aliases[u.ID],
		})
	}

	return connect.NewResponse(resp), nil
}
