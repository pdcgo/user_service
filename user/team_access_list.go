package user

import (
	"context"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/db_models"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/user_models"
)

// TeamAccessList implements [user_ifaceconnect.V2UserServiceHandler]. It returns
// the teams the user belongs to — team name + type + the user's role and alias —
// read from user_team_roles joined to teams. Authenticated callers only; when
// user_id is 0 the authenticated caller's own teams are returned.
func (s *v2UserServiceImpl) TeamAccessList(
	ctx context.Context,
	req *connect.Request[user_iface.TeamAccessListRequest],
) (*connect.Response[user_iface.TeamAccessListResponse], error) {
	caller, err := access_interceptors.GetIdentityFromCtx(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	userID := uint64(caller.IdentityId)
	if req.Msg.UserId != 0 {
		userID = req.Msg.UserId
	}

	db := s.db.WithContext(ctx)

	var rows []struct {
		TeamID   uint
		TeamName string
		TeamType db_models.TeamType
		Alias    string
		Role     uint
	}
	if err := db.
		Model(&user_models.UserTeamRole{}).
		Select("user_team_roles.team_id, teams.name AS team_name, teams.type AS team_type, user_team_roles.alias, user_team_roles.role").
		Joins("JOIN teams ON teams.id = user_team_roles.team_id").
		Where("user_team_roles.user_id = ?", userID).
		Order("user_team_roles.team_id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	resp := &user_iface.TeamAccessListResponse{
		Access: make([]*user_iface.TeamAccessItem, 0, len(rows)),
	}
	for _, r := range rows {
		resp.Access = append(resp.Access, &user_iface.TeamAccessItem{
			TeamId:   uint64(r.TeamID),
			TeamName: r.TeamName,
			TeamType: teamTypeToProto(r.TeamType),
			Alias:    r.Alias,
			Role:     role_base.Role(r.Role),
		})
	}

	return connect.NewResponse(resp), nil
}

// teamTypeToProto maps the stored team type string to the proto enum. The proto
// has no ROOT variant, so root teams are reported as ADMIN.
func teamTypeToProto(t db_models.TeamType) user_iface.TeamType {
	switch t {
	case db_models.WarehouseTeamType:
		return user_iface.TeamType_TEAM_TYPE_WAREHOUSE
	case db_models.SellingTeamType:
		return user_iface.TeamType_TEAM_TYPE_SELLING
	case db_models.AdminTeamType, db_models.RootTeamType:
		return user_iface.TeamType_TEAM_TYPE_ADMIN
	default:
		return user_iface.TeamType_TEAM_TYPE_UNSPECIFIED
	}
}
