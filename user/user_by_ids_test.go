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
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 5, UserID: alice.ID, Role: role_base.Role_ROLE_TEAM_OWNER, Alias: "boss"}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 5, UserID: bob.ID, Role: role_base.Role_ROLE_TEAM_ADMIN, Alias: "deputy"}).Error)

				svc := user.NewV2UserService(tx)
				ctx := context.Background()
				ids := []uint64{uint64(alice.ID), uint64(bob.ID), uint64(carol.ID)}

				t.Run("no team_id: identity only, alias nil", func(t *testing.T) {
					res, err := svc.UserByIDs(ctx, connect.NewRequest(&user_iface.UserByIDsRequest{Ids: ids}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 3)

					a := res.Msg.Users[uint64(alice.ID)]
					assert.NotNil(t, a)
					assert.Equal(t, "alice", a.User.Username)
					assert.Equal(t, "pic-alice", a.User.ProfilePicture)
					assert.Equal(t, user_iface.UserStatus_USER_STATUS_ACTIVE, a.User.Status)
					assert.Nil(t, a.Alias)

					// suspended status round-trips
					assert.Equal(t, user_iface.UserStatus_USER_STATUS_SUSPENDED, res.Msg.Users[uint64(bob.ID)].User.Status)
				})

				t.Run("team_id set: members carry alias + role, non-member nil", func(t *testing.T) {
					res, err := svc.UserByIDs(ctx, connect.NewRequest(&user_iface.UserByIDsRequest{Ids: ids, TeamId: 5}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 3)

					a := res.Msg.Users[uint64(alice.ID)].Alias
					assert.NotNil(t, a)
					assert.Equal(t, uint64(5), a.TeamId)
					assert.Equal(t, "boss", a.Alias)
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, a.Role)

					b := res.Msg.Users[uint64(bob.ID)].Alias
					assert.NotNil(t, b)
					assert.Equal(t, "deputy", b.Alias)
					assert.Equal(t, role_base.Role_ROLE_TEAM_ADMIN, b.Role)

					// carol is not a member of team 5
					assert.Nil(t, res.Msg.Users[uint64(carol.ID)].Alias)
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
