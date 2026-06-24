package user_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	user_iface "github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/schema/services/user_iface/v2/user_ifaceconnect"
	"github.com/pdcgo/shared/db_models"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/san_collection/san_verification"
	"github.com/pdcgo/user_service/user"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// Minimal stand-ins for the legacy tables the sync reads. Column/table names
// match the real authorization_iface.Role / UserRole and db_models.Team so the
// handler's raw join exercises the same query, without pulling their full
// association graphs into AutoMigrate.
type legacyRoleSeed struct {
	ID       uint `gorm:"primarykey"`
	Key      string
	DomainID uint
}

func (legacyRoleSeed) TableName() string { return "roles" }

type legacyUserRoleSeed struct {
	ID     uint `gorm:"primarykey"`
	RoleID uint
	UserID uint
}

func (legacyUserRoleSeed) TableName() string { return "user_roles" }

type teamSeed struct {
	ID   uint `gorm:"primarykey"`
	Type db_models.TeamType
}

func (teamSeed) TableName() string { return "teams" }

func TestTeamSynclegacy(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "team sync legacy",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(
					&teamSeed{},
					&legacyRoleSeed{},
					&legacyUserRoleSeed{},
					&user_models.UserTeamRole{},
				))

				// Real RPC, exercised through an in-process connect server so the
				// server-stream path is covered end to end.
				svc := user.NewV2UserService(tx, san_verification.NewMockOtpVerification())
				mux := http.NewServeMux()
				mux.Handle(user_ifaceconnect.NewV2UserServiceHandler(svc))
				srv := httptest.NewServer(mux)
				defer srv.Close()
				client := user_ifaceconnect.NewV2UserServiceClient(srv.Client(), srv.URL)
				ctx := context.Background()

				// sync runs TeamSynclegacy and returns how many stream messages
				// (one heartbeat per synced assignment) arrived.
				sync := func(teamID uint64) (int, error) {
					stream, err := client.TeamSynclegacy(ctx,
						connect.NewRequest(&user_iface.TeamSynclegacyRequest{TeamId: teamID}))
					if err != nil {
						return 0, err
					}
					defer stream.Close()
					count := 0
					for stream.Receive() {
						count++
					}
					return count, stream.Err()
				}

				rolesByUser := func(teamID uint) map[uint]role_base.Role {
					var rows []user_models.UserTeamRole
					assert.NoError(t, tx.Where("team_id = ?", teamID).Find(&rows).Error)
					out := map[uint]role_base.Role{}
					for _, r := range rows {
						out[r.UserID] = r.Role
					}
					return out
				}

				t.Run("selling team maps keys and seeds roles", func(t *testing.T) {
					const team = uint(7)
					assert.NoError(t, tx.Create(&teamSeed{ID: team, Type: db_models.SellingTeamType}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 1, Key: "owner", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 2, Key: "admin", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 3, Key: "cs", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 1, UserID: 101}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 2, UserID: 102}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 3, UserID: 103}).Error)

					count, err := sync(uint64(team))
					assert.NoError(t, err)
					assert.Equal(t, 3, count)

					got := rolesByUser(team)
					assert.Equal(t, role_base.Role_ROLE_TEAM_OWNER, got[101])
					assert.Equal(t, role_base.Role_ROLE_TEAM_ADMIN, got[102])
					assert.Equal(t, role_base.Role_ROLE_TEAM_CUSTOMER_SERVICE, got[103])
				})

				t.Run("warehouse team mapping is type-aware", func(t *testing.T) {
					const team = uint(8)
					assert.NoError(t, tx.Create(&teamSeed{ID: team, Type: db_models.WarehouseTeamType}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 10, Key: "owner", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 11, Key: "admin", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 12, Key: "packer", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 10, UserID: 201}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 11, UserID: 202}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 12, UserID: 203}).Error)

					count, err := sync(uint64(team))
					assert.NoError(t, err)
					assert.Equal(t, 3, count)

					got := rolesByUser(team)
					assert.Equal(t, role_base.Role_ROLE_WAREHOUSE_OWNER, got[201])
					assert.Equal(t, role_base.Role_ROLE_WAREHOUSE_ADMIN, got[202])
					assert.Equal(t, role_base.Role_ROLE_WAREHOUSE_STAFF, got[203])
				})

				t.Run("re-running is idempotent", func(t *testing.T) {
					// team 7 already synced above; running again must not duplicate.
					count, err := sync(7)
					assert.NoError(t, err)
					assert.Equal(t, 3, count)

					var n int64
					assert.NoError(t, tx.Model(&user_models.UserTeamRole{}).
						Where("team_id = ?", 7).Count(&n).Error)
					assert.Equal(t, int64(3), n)
				})

				t.Run("unmapped key aborts and writes nothing", func(t *testing.T) {
					const team = uint(9)
					assert.NoError(t, tx.Create(&teamSeed{ID: team, Type: db_models.SellingTeamType}).Error)
					assert.NoError(t, tx.Create(&legacyRoleSeed{ID: 20, Key: "wizard", DomainID: team}).Error)
					assert.NoError(t, tx.Create(&legacyUserRoleSeed{RoleID: 20, UserID: 301}).Error)

					count, err := sync(uint64(team))
					assert.Error(t, err)
					assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
					assert.Zero(t, count)

					var n int64
					assert.NoError(t, tx.Model(&user_models.UserTeamRole{}).
						Where("team_id = ?", team).Count(&n).Error)
					assert.Zero(t, n)
				})
			})
		},
	)
}
