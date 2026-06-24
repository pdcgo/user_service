package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
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

func TestUserByIDs(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "user by ids",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))

				alice := &user_models.User{Username: "alice", Email: "alice@x.com", Name: "Alice", ProfilePicture: "pic-alice"}
				bob := &user_models.User{Username: "bob", Email: "bob@x.com", Name: "Bob", IsSuspended: true}
				carol := &user_models.User{Username: "carol", Email: "carol@x.com", Name: "Carol"}
				for _, u := range []*user_models.User{alice, bob, carol} {
					assert.NoError(t, tx.Create(u).Error)
				}
				// team 5: alice = owner "boss", bob = admin "deputy"; carol is not a member.
				// alice is also in team 6 (owner "chief") to exercise the multi-team path.
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 5, UserID: alice.ID, Role: role_base.Role_ROLE_TEAM_OWNER, Alias: "boss"}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 6, UserID: alice.ID, Role: role_base.Role_ROLE_TEAM_OWNER, Alias: "chief"}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 5, UserID: bob.ID, Role: role_base.Role_ROLE_TEAM_ADMIN, Alias: "deputy"}).Error)

				svc := user.NewV2UserService(tx, san_verification.NewMockOtpVerification())
				ctx := context.Background()
				ids := []uint64{uint64(alice.ID), uint64(bob.ID), uint64(carol.ID)}

				t.Run("no team_id: all team aliases per user", func(t *testing.T) {
					res, err := svc.UserByIDs(ctx, connect.NewRequest(&user_iface.UserByIDsRequest{Ids: ids}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 3)

					a := res.Msg.Users[uint64(alice.ID)]
					assert.NotNil(t, a)
					assert.Equal(t, "alice", a.User.Username)
					assert.Equal(t, "pic-alice", a.User.ProfilePicture)
					assert.Equal(t, user_iface.UserStatus_USER_STATUS_ACTIVE, a.User.Status)
					// alice carries both teams, ordered by team_id (5 then 6)
					assert.Len(t, a.Alias, 2)
					assert.Equal(t, uint64(5), a.Alias[0].TeamId)
					assert.Equal(t, "boss", a.Alias[0].Alias)
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, a.Alias[0].Role)
					assert.Equal(t, uint64(6), a.Alias[1].TeamId)
					assert.Equal(t, "chief", a.Alias[1].Alias)

					// bob is in one team; carol is in none
					assert.Len(t, res.Msg.Users[uint64(bob.ID)].Alias, 1)
					assert.Empty(t, res.Msg.Users[uint64(carol.ID)].Alias)

					// suspended status round-trips
					assert.Equal(t, user_iface.UserStatus_USER_STATUS_SUSPENDED, res.Msg.Users[uint64(bob.ID)].User.Status)
				})

				t.Run("team_id set: members carry alias + role, non-member empty", func(t *testing.T) {
					res, err := svc.UserByIDs(ctx, connect.NewRequest(&user_iface.UserByIDsRequest{Ids: ids, TeamId: 5}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 3)

					a := res.Msg.Users[uint64(alice.ID)].Alias
					assert.Len(t, a, 1)
					assert.Equal(t, uint64(5), a[0].TeamId)
					assert.Equal(t, "boss", a[0].Alias)
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, a[0].Role)

					b := res.Msg.Users[uint64(bob.ID)].Alias
					assert.Len(t, b, 1)
					assert.Equal(t, "deputy", b[0].Alias)
					assert.Equal(t, role_base.Role_ROLE_TEAM_ADMIN, b[0].Role)

					// carol is not a member of team 5
					assert.Empty(t, res.Msg.Users[uint64(carol.ID)].Alias)
				})

				t.Run("missing id is omitted", func(t *testing.T) {
					res, err := svc.UserByIDs(ctx, connect.NewRequest(&user_iface.UserByIDsRequest{Ids: []uint64{uint64(alice.ID), 9999}}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.NotNil(t, res.Msg.Users[uint64(alice.ID)])
				})
			})
		})
}
