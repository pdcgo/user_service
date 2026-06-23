package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/user"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// testTeam is a minimal mapping of the columns TeamAccessList reads from the
// teams table, so the test doesn't have to migrate the full db_models.Team
// (whose TeamInfo association would cascade into Warehouse/User tables).
type testTeam struct {
	ID   uint `gorm:"primarykey"`
	Type string
	Name string
}

func (testTeam) TableName() string { return "teams" }

// TestTeamAccessList covers the handler: it returns the authenticated user's
// teams (name + type + role + alias), ordered by team_id; rejects unauthenticated
// callers; and returns an empty list for a user with no memberships.
func TestTeamAccessList(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "team access list",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(
					&user_models.User{},
					&user_models.UserTeamRole{},
					&testTeam{},
				))

				alice := &user_models.User{Username: "alice", Email: "alice@x.com", Name: "Alice"}
				bob := &user_models.User{Username: "bob", Email: "bob@x.com", Name: "Bob"}
				assert.NoError(t, tx.Create(alice).Error)
				assert.NoError(t, tx.Create(bob).Error)

				assert.NoError(t, tx.Create(&testTeam{ID: 10, Type: "warehouse", Name: "Acme WH"}).Error)
				assert.NoError(t, tx.Create(&testTeam{ID: 20, Type: "selling", Name: "Beta Sell"}).Error)

				// alice is in both teams; bob is in none.
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 10, UserID: alice.ID, Role: role_base.Role_ROLE_WAREHOUSE_OWNER, Alias: "a10"}).Error)
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{TeamID: 20, UserID: alice.ID, Role: role_base.Role_ROLE_TEAM_ADMIN, Alias: "a20"}).Error)

				svc := user.NewV2UserService(tx)

				aliceCtx := access_interceptors.SetIdentityToCtx(
					context.Background(),
					&role_base.Identity{IdentityId: uint32(alice.ID)},
				)

				t.Run("returns the caller's teams ordered by team_id", func(t *testing.T) {
					res, err := svc.TeamAccessList(aliceCtx, connect.NewRequest(&user_iface.TeamAccessListRequest{}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Access, 2)

					a := res.Msg.Access[0]
					assert.Equal(t, uint64(10), a.TeamId)
					assert.Equal(t, "Acme WH", a.TeamName)
					assert.Equal(t, user_iface.TeamType_TEAM_TYPE_WAREHOUSE, a.TeamType)
					assert.Equal(t, "a10", a.Alias)
					assert.Equal(t, role_base.Role_ROLE_WAREHOUSE_OWNER, a.Role)

					b := res.Msg.Access[1]
					assert.Equal(t, uint64(20), b.TeamId)
					assert.Equal(t, "Beta Sell", b.TeamName)
					assert.Equal(t, user_iface.TeamType_TEAM_TYPE_SELLING, b.TeamType)
					assert.Equal(t, role_base.Role_ROLE_TEAM_ADMIN, b.Role)
				})

				t.Run("unauthenticated caller is rejected", func(t *testing.T) {
					_, err := svc.TeamAccessList(context.Background(), connect.NewRequest(&user_iface.TeamAccessListRequest{}))
					assert.Error(t, err)
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("user with no teams gets an empty list", func(t *testing.T) {
					bobCtx := access_interceptors.SetIdentityToCtx(
						context.Background(),
						&role_base.Identity{IdentityId: uint32(bob.ID)},
					)
					res, err := svc.TeamAccessList(bobCtx, connect.NewRequest(&user_iface.TeamAccessListRequest{}))
					assert.NoError(t, err)
					assert.Empty(t, res.Msg.Access)
				})
			})
		},
	)
}
