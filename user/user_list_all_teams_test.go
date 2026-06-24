package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	common "github.com/pdcgo/schema/services/common/v1"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/san_collection/san_verification"
	"github.com/pdcgo/user_service/user"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// TestUserListAllTeams covers the team_id == 0 ("all teams") path of UserList:
// all users (teamless included), each carrying a UserAlias for every team they
// belong to.
func TestUserListAllTeams(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "user list all teams",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))

				dave := &user_models.User{Username: "dave", Email: "dave@x.com", Name: "Dave"}
				erin := &user_models.User{Username: "erin", Email: "erin@x.com", Name: "Erin"}
				frank := &user_models.User{Username: "frank", Email: "frank@x.com", Name: "Frank"}
				for _, u := range []*user_models.User{dave, erin, frank} {
					assert.NoError(t, tx.Create(u).Error)
				}
				// dave: team 10 owner + team 20 admin; erin: team 10 CS; frank: no team.
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 10, UserID: dave.ID, Role: role_base.Role_ROLE_TEAM_OWNER, Alias: "d10"}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 20, UserID: dave.ID, Role: role_base.Role_ROLE_TEAM_ADMIN, Alias: "d20"}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 10, UserID: erin.ID, Role: role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE, Alias: "e10"}).Error)

				svc := user.NewV2UserService(tx, san_verification.NewMockOtpVerification())
				ctx := context.Background()

				byID := func(res []*user_iface.UserMapItem) map[uint64]*user_iface.UserMapItem {
					m := map[uint64]*user_iface.UserMapItem{}
					for _, u := range res {
						m[u.User.Id] = u
					}
					return m
				}

				t.Run("team_id 0: all users, all their team aliases", func(t *testing.T) {
					res, err := svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{TeamId: 0, Page: &common.PageFilter{Page: 1, Limit: 10}}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 3)
					m := byID(res.Msg.Users)

					// dave: both teams, ordered by team_id (10 then 20)
					d := m[uint64(dave.ID)].Alias
					assert.Len(t, d, 2)
					assert.Equal(t, uint64(10), d[0].TeamId)
					assert.Equal(t, "d10", d[0].Alias)
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, d[0].Role)
					assert.Equal(t, uint64(20), d[1].TeamId)
					assert.Equal(t, role_base.Role_ROLE_TEAM_ADMIN, d[1].Role)

					// erin: one team; frank: teamless (empty alias)
					assert.Len(t, m[uint64(erin.ID)].Alias, 1)
					assert.Empty(t, m[uint64(frank.ID)].Alias)
				})

				t.Run("team_id 0 + role filter: matched users keep all their aliases", func(t *testing.T) {
					// only dave holds OWNER (in team 10); he still carries both aliases.
					res, err := svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{
						TeamId: 0,
						Role:   role_base.Role_ROLE_TEAM_OWNER,
						Page:   &common.PageFilter{Page: 1, Limit: 10},
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(dave.ID), res.Msg.Users[0].User.Id)
					assert.Len(t, res.Msg.Users[0].Alias, 2)
				})

				t.Run("team_id 0 + q filter matches one user", func(t *testing.T) {
					res, err := svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{
						TeamId: 0,
						Q:      "erin",
						Page:   &common.PageFilter{Page: 1, Limit: 10},
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(erin.ID), res.Msg.Users[0].User.Id)
				})

				t.Run("pagination splits the all-teams list", func(t *testing.T) {
					p1, err := svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{
						TeamId: 0,
						Page:   &common.PageFilter{Page: 1, Limit: 2},
					}))
					assert.NoError(t, err)
					assert.Len(t, p1.Msg.Users, 2)
					assert.Equal(t, int64(3), p1.Msg.PageInfo.TotalItems)
					assert.Equal(t, int64(2), p1.Msg.PageInfo.TotalPage)
					// ordered by user id asc: dave then erin on page 1
					assert.Equal(t, uint64(dave.ID), p1.Msg.Users[0].User.Id)
					assert.Equal(t, uint64(erin.ID), p1.Msg.Users[1].User.Id)

					p2, err := svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{
						TeamId: 0,
						Page:   &common.PageFilter{Page: 2, Limit: 2},
					}))
					assert.NoError(t, err)
					assert.Len(t, p2.Msg.Users, 1)
					assert.Equal(t, uint64(frank.ID), p2.Msg.Users[0].User.Id)
				})

				t.Run("team_id set keeps the single team-scoped path", func(t *testing.T) {
					res, err := svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{TeamId: 10, Page: &common.PageFilter{Page: 1, Limit: 10}}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 2) // dave + erin, not frank
					m := byID(res.Msg.Users)
					// each member carries exactly one alias, scoped to team 10
					assert.Len(t, m[uint64(dave.ID)].Alias, 1)
					assert.Equal(t, uint64(10), m[uint64(dave.ID)].Alias[0].TeamId)
					assert.Equal(t, "d10", m[uint64(dave.ID)].Alias[0].Alias)
					assert.Len(t, m[uint64(erin.ID)].Alias, 1)
					assert.Equal(t, uint64(10), m[uint64(erin.ID)].Alias[0].TeamId)
				})
			})
		},
	)
}
