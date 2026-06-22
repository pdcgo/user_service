package user

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/db_models"
)

// legacyRoleToEnum maps a legacy `roles.key` value to the new role_base.v1.Role
// enum. The same key means different roles per team type (e.g. "owner" is a
// team owner for a selling team but a warehouse owner for a warehouse team), so
// the mapping is scoped by the team's Type. The bool is false when the
// (team type, key) pair has no mapping.
func legacyRoleToEnum(teamType db_models.TeamType, key string) (role_base.Role, bool) {
	switch teamType {
	case db_models.RootTeamType:
		switch key {
		case "system":
			return role_base.Role_ROLE_SYSTEM, true
		case "root":
			return role_base.Role_ROLE_ROOT, true
		}
	case db_models.AdminTeamType:
		switch key {
		case "owner", "admin":
			return role_base.Role_ROLE_TEAM_ADMIN, true
		}
	case db_models.SellingTeamType:
		switch key {
		case "owner":
			return role_base.Role_ROLE_TEAM_OWNER, true
		case "admin":
			return role_base.Role_ROLE_TEAM_ADMIN, true
		case "cs":
			return role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE, true
		}
	case db_models.WarehouseTeamType:
		switch key {
		case "owner":
			return role_base.Role_ROLE_WAREHOUSE_OWNER, true
		case "admin":
			return role_base.Role_ROLE_WAREHOUSE_ADMIN, true
		case "packer":
			return role_base.Role_ROLE_WAREHOUSE_STAFF, true
		}
	}
	return role_base.Role_ROLE_UNSPECIFIED, false
}

// legacyRoleRow is a row of the legacy role join, one per (user, role) for the
// requested team (domain).
type legacyRoleRow struct {
	Role   string
	UserID uint64
	TeamID uint64
	Alias  string
}

// TeamSynclegacy implements [user_ifaceconnect.V2UserServiceHandler]. It reads
// the legacy role assignments for a single team (the legacy `user_roles`/`roles`
// tables, scoped by domain_id == team_id) and seeds them into the new role
// system via TeamUserUpdate. Mapping is team-type-aware; any legacy role key it
// cannot map aborts the sync (reported with the full list) before anything is
// written, so the mapping/proto can be fixed and the sync re-run. TeamUserUpdate
// upserts, so re-running is safe.
func (s *v2UserServiceImpl) TeamSynclegacy(
	ctx context.Context,
	req *connect.Request[user_iface.TeamSynclegacyRequest],
	stream *connect.ServerStream[user_iface.TeamSynclegacyResponse],
) error {
	db := s.db.WithContext(ctx)
	teamID := req.Msg.TeamId

	// Resolve the team type to drive the role-key mapping.
	var team db_models.Team
	err := db.Select("id", "type").First(&team, "id = ?", teamID).Error
	if err != nil {
		return err
	}

	// 1. Query legacy role data for this team (domain).
	var rows []legacyRoleRow
	err = db.
		Table("user_roles AS ur").
		Select([]string{
			"r.key as role",
			"r.domain_id as team_id",
			"ur.user_id as user_id",
			"ut.alias",
		}).
		Joins("left join roles r on r.id = ur.role_id").
		Joins("left join user_teams ut on ut.team_id = r.domain_id and ut.user_id = ur.user_id").
		Where("r.domain_id = ?", teamID).
		Scan(&rows).
		Error
	if err != nil {
		return err
	}

	for _, row := range rows {
		role, ok := legacyRoleToEnum(team.Type, row.Role)
		if !ok {
			slog.Error("cannot map permission", "user", row)
			continue
		}

		_, err = s.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
			TeamId: row.TeamID,
			Action: &user_iface.TeamUserUpdateRequest_Add{
				Add: &user_iface.AddUser{
					UserId: row.TeamID,
					Role:   role,
					Alias:  row.Alias,
				},
			},
		}))

		if err != nil {
			return err
		}

		// Emit a heartbeat per synced assignment so the caller sees progress.
		err = stream.Send(&user_iface.TeamSynclegacyResponse{})
		if err != nil {
			return err
		}
	}

	return nil
}
