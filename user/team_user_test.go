package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/user_service/user"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestTeamUser(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "team user",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))
				svc := user.NewV2UserService(tx)
				ctx := context.Background()

				u1 := &user_models.User{Username: "alice", Email: "alice@x.com", Name: "Alice"}
				u2 := &user_models.User{Username: "bob", Email: "bob@x.com", Name: "Bob"}
				assert.NoError(t, tx.Create(u1).Error)
				assert.NoError(t, tx.Create(u2).Error)

				const teamID = 7

				addReq := func(userID uint, role role_base.Role) *connect.Request[user_iface.TeamUserUpdateRequest] {
					return connect.NewRequest(&user_iface.TeamUserUpdateRequest{
						TeamId: teamID,
						Action: &user_iface.TeamUserUpdateRequest_Add{
							Add: &user_iface.AddUser{UserId: uint64(userID), Role: role},
						},
					})
				}

				t.Run("add upserts role", func(t *testing.T) {
					_, err := svc.TeamUserUpdate(ctx, addReq(u1.ID, role_base.Role_ROLE_TEAM_ADMIN))
					assert.NoError(t, err)

					// adding again changes the role rather than duplicating
					_, err = svc.TeamUserUpdate(ctx, addReq(u1.ID, role_base.Role_ROLE_TEAM_OWNER))
					assert.NoError(t, err)

					var rows []user_models.UserTeamRole
					assert.NoError(t, tx.Where("team_id = ? AND user_id = ?", teamID, u1.ID).Find(&rows).Error)
					assert.Len(t, rows, 1)
					assert.Equal(t, uint(role_base.Role_ROLE_TEAM_OWNER), rows[0].Role)
				})

				t.Run("list returns team members with roles + filters", func(t *testing.T) {
					_, err := svc.TeamUserUpdate(ctx, addReq(u2.ID, role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE))
					assert.NoError(t, err)

					// unfiltered: both members, each carrying their team role
					res, err := svc.TeamUserList(ctx,
						connect.NewRequest(&user_iface.TeamUserListRequest{TeamId: teamID}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 2)
					roleByID := map[uint64]role_base.Role{}
					for _, u := range res.Msg.Users {
						roleByID[u.Id] = u.Role
					}
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, roleByID[uint64(u1.ID)])
					assert.Equal(t, role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE, roleByID[uint64(u2.ID)])

					// role filter -> only the team owner (u1)
					res, err = svc.TeamUserList(ctx, connect.NewRequest(&user_iface.TeamUserListRequest{
						TeamId: teamID,
						Role:   role_base.Role_ROLE_TEAM_OWNER,
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u1.ID), res.Msg.Users[0].Id)

					// q filter -> matches u2 (username "bob")
					res, err = svc.TeamUserList(ctx, connect.NewRequest(&user_iface.TeamUserListRequest{
						TeamId: teamID,
						Q:      "bob",
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u2.ID), res.Msg.Users[0].Id)
				})

				t.Run("remove drops the member", func(t *testing.T) {
					_, err := svc.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
						TeamId: teamID,
						Action: &user_iface.TeamUserUpdateRequest_Remove{
							Remove: &user_iface.RemoveUser{UserId: uint64(u1.ID)},
						},
					}))
					assert.NoError(t, err)

					res, err := svc.TeamUserList(ctx,
						connect.NewRequest(&user_iface.TeamUserListRequest{TeamId: teamID}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u2.ID), res.Msg.Users[0].Id)
				})

				t.Run("empty oneof is invalid argument", func(t *testing.T) {
					_, err := svc.TeamUserUpdate(ctx,
						connect.NewRequest(&user_iface.TeamUserUpdateRequest{TeamId: teamID}))
					assert.Error(t, err)
					assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
				})
			})
		})
}
