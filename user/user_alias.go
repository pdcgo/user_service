package user

import (
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
	"gorm.io/gorm"
)

// loadTeamAliases returns, per user id, their team aliases (alias + role per
// team) as UserAlias entries. When teamID != 0 it is restricted to that single
// team (so each user has at most one entry); when teamID == 0 it returns every
// team each user belongs to. Each entry carries the row's own team_id. Users
// with no matching membership are absent from the map, which callers treat as an
// empty alias list. Ordered by (user_id, team_id) for deterministic output.
func loadTeamAliases(db *gorm.DB, userIDs []uint, teamID uint64) (map[uint][]*user_iface.UserAlias, error) {
	out := map[uint][]*user_iface.UserAlias{}
	if len(userIDs) == 0 {
		return out, nil
	}

	q := db.Model(&user_models.UserTeamRole{}).Where("user_id IN ?", userIDs)
	if teamID != 0 {
		q = q.Where("team_id = ?", teamID)
	}

	var rows []user_models.UserTeamRole
	if err := q.Order("user_id ASC, team_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	for _, r := range rows {
		out[r.UserID] = append(out[r.UserID], &user_iface.UserAlias{
			TeamId: uint64(r.TeamID),
			Alias:  r.Alias,
			Role:   r.Role,
		})
	}
	return out, nil
}
