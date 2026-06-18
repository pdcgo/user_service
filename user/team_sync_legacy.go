package user

import (
	"context"
	"fmt"
	"sort"

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
	UserID uint
	TeamID uint
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
		Select("r.key AS role, ur.user_id AS user_id, r.domain_id AS team_id").
		Joins("LEFT JOIN roles r ON r.id = ur.role_id").
		Where("r.domain_id = ?", teamID).
		Scan(&rows).
		Error
	if err != nil {
		return err
	}

	// Resolve every row first; collect any keys we cannot map so we report them
	// all at once and write nothing on failure.
	type seedItem struct {
		userID uint
		role   role_base.Role
	}
	seeds := make([]seedItem, 0, len(rows))
	unmapped := map[string]struct{}{}
	for _, row := range rows {
		role, ok := legacyRoleToEnum(team.Type, row.Role)
		if !ok {
			unmapped[row.Role] = struct{}{}
			continue
		}
		seeds = append(seeds, seedItem{userID: row.UserID, role: role})
	}

	if len(unmapped) > 0 {
		keys := make([]string, 0, len(unmapped))
		for k := range unmapped {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return connect.NewError(
			connect.CodeFailedPrecondition,
			fmt.Errorf("team %d (type %q) has unmapped legacy role keys %v: add a mapping/proto enum before syncing", teamID, team.Type, keys),
		)
	}

	// 2. Seed each assignment into the new role system. TeamUserUpdate upserts
	// on (team_id, user_id), so re-running the sync is idempotent.
	for _, seed := range seeds {
		_, err = s.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
			TeamId: teamID,
			Action: &user_iface.TeamUserUpdateRequest_Add{
				Add: &user_iface.AddUser{
					UserId: uint64(seed.userID),
					Role:   seed.role,
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
