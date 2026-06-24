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

func TestTeamUser(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "team user",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))
				svc := user.NewV2UserService(tx, san_verification.NewMockOtpVerification())
				ctx := context.Background()

				u1 := &user_models.User{Username: "alice", Email: "alice@x.com", Name: "Alice", ProfilePicture: "pic-alice"}
				u2 := &user_models.User{Username: "bob", Email: "bob@x.com", Name: "Bob", ProfilePicture: "pic-bob"}
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
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, rows[0].Role)
				})

				t.Run("add stores and upserts alias", func(t *testing.T) {
					const aliasTeam = 99
					addAlias := func(alias string) {
						_, err := svc.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
							TeamId: aliasTeam,
							Action: &user_iface.TeamUserUpdateRequest_Add{
								Add: &user_iface.AddUser{UserId: uint64(u1.ID), Role: role_base.Role_ROLE_TEAM_ADMIN, Alias: alias},
							},
						}))
						assert.NoError(t, err)
					}
					aliasOf := func() string {
						var rec user_models.UserTeamRole
						assert.NoError(t, tx.Where("team_id = ? AND user_id = ?", uint(aliasTeam), u1.ID).First(&rec).Error)
						return rec.Alias
					}

					addAlias("boss")
					assert.Equal(t, "boss", aliasOf())

					// re-adding upserts the alias
					addAlias("chief")
					assert.Equal(t, "chief", aliasOf())
				})

				t.Run("create makes a new user and adds them to the team", func(t *testing.T) {
					const createTeam = 88
					_, err := svc.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
						TeamId: createTeam,
						Action: &user_iface.TeamUserUpdateRequest_Create{
							Create: &user_iface.CreateUser{
								Email:    "newbie@x.com",
								Username: "newbie",
								Password: "secret123",
								Name:     "Newbie",
								Role:     role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE,
								Alias:    "rookie",
							},
						},
					}))
					assert.NoError(t, err)

					// user created with a hashed (non-plaintext) password
					var u user_models.User
					assert.NoError(t, tx.Where("username = ?", "newbie").First(&u).Error)
					assert.Equal(t, "newbie@x.com", u.Email)
					assert.NotEmpty(t, u.Password)
					assert.NotEqual(t, "secret123", u.Password)

					// and added to the team with role + alias
					var rec user_models.UserTeamRole
					assert.NoError(t, tx.Where("team_id = ? AND user_id = ?", uint(createTeam), u.ID).First(&rec).Error)
					assert.Equal(t, role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE, rec.Role)
					assert.Equal(t, "rookie", rec.Alias)

					t.Run("duplicate username is already-exists and rolls back", func(t *testing.T) {
						_, err := svc.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
							TeamId: createTeam,
							Action: &user_iface.TeamUserUpdateRequest_Create{
								Create: &user_iface.CreateUser{
									Email:    "other@x.com",
									Username: "newbie", // collides with the user created above
									Password: "secret123",
									Name:     "Other",
									Role:     role_base.Role_ROLE_TEAM_ADMIN,
								},
							},
						}))
						assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))

						// the failed create rolled back: no user for other@x.com
						var n int64
						assert.NoError(t, tx.Model(&user_models.User{}).Where("email = ?", "other@x.com").Count(&n).Error)
						assert.Equal(t, int64(0), n)
					})
				})

				t.Run("list returns team members with roles + filters", func(t *testing.T) {
					_, err := svc.TeamUserUpdate(ctx, addReq(u2.ID, role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE))
					assert.NoError(t, err)

					// unfiltered: both members, each carrying a single team-scoped alias+role
					res, err := svc.UserList(ctx,
						connect.NewRequest(&user_iface.UserListRequest{TeamId: teamID, Page: &common.PageFilter{Page: 1, Limit: 10}}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 2)
					roleByID := map[uint64]role_base.Role{}
					picByID := map[uint64]string{}
					for _, u := range res.Msg.Users {
						assert.Len(t, u.Alias, 1)
						assert.Equal(t, uint64(teamID), u.Alias[0].TeamId)
						roleByID[u.User.Id] = u.Alias[0].Role
						picByID[u.User.Id] = u.User.ProfilePicture
					}
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, roleByID[uint64(u1.ID)])
					assert.Equal(t, role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE, roleByID[uint64(u2.ID)])
					assert.Equal(t, "pic-alice", picByID[uint64(u1.ID)])
					assert.Equal(t, "pic-bob", picByID[uint64(u2.ID)])

					// role filter -> only the team owner (u1)
					res, err = svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{
						TeamId: teamID,
						Role:   role_base.Role_ROLE_TEAM_OWNER,
						Page:   &common.PageFilter{Page: 1, Limit: 10},
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u1.ID), res.Msg.Users[0].User.Id)

					// q filter -> matches u2 (username "bob")
					res, err = svc.UserList(ctx, connect.NewRequest(&user_iface.UserListRequest{
						TeamId: teamID,
						Q:      "bob",
						Page:   &common.PageFilter{Page: 1, Limit: 10},
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u2.ID), res.Msg.Users[0].User.Id)
				})

				t.Run("remove drops the member", func(t *testing.T) {
					_, err := svc.TeamUserUpdate(ctx, connect.NewRequest(&user_iface.TeamUserUpdateRequest{
						TeamId: teamID,
						Action: &user_iface.TeamUserUpdateRequest_Remove{
							Remove: &user_iface.RemoveUser{UserId: uint64(u1.ID)},
						},
					}))
					assert.NoError(t, err)

					res, err := svc.UserList(ctx,
						connect.NewRequest(&user_iface.UserListRequest{TeamId: teamID, Page: &common.PageFilter{Page: 1, Limit: 10}}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u2.ID), res.Msg.Users[0].User.Id)
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
