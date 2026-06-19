package auth_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_caches"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/user_service/auth"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestCheckAccessTeamRole(t *testing.T) {
	const secret = "test-secret"
	const teamID = 7

	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "check access team role",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.UserTeamRole{}))
				svc := auth.NewV2AuthService(tx, secret, san_caches.NewSkipCacheManager())
				ctx := context.Background()

				// makeToken (check_access_test.go) signs a token for user 42.
				token := makeToken(t, secret, time.Now().Add(time.Hour))
				assert.NoError(t, tx.Create(&user_models.UserTeamRole{
					UserID: 42,
					TeamID: teamID,
					Role:   uint(role_base.Role_ROLE_TEAM_ADMIN),
				}).Error)

				t.Run("team_id set returns the caller's role", func(t *testing.T) {
					res, err := svc.CheckAccess(ctx, connect.NewRequest(&user_iface.CheckAccessRequest{
						Token:  token,
						TeamId: teamID,
					}))
					assert.NoError(t, err)
					assert.Equal(t, role_base.Role_ROLE_TEAM_ADMIN, res.Msg.Role)
				})

				t.Run("non-member team returns ROLE_UNSPECIFIED", func(t *testing.T) {
					res, err := svc.CheckAccess(ctx, connect.NewRequest(&user_iface.CheckAccessRequest{
						Token:  token,
						TeamId: 999,
					}))
					assert.NoError(t, err)
					assert.Equal(t, role_base.Role_ROLE_UNSPECIFIED, res.Msg.Role)
				})

				t.Run("no team_id leaves role unspecified", func(t *testing.T) {
					res, err := svc.CheckAccess(ctx, connect.NewRequest(&user_iface.CheckAccessRequest{
						Token: token,
					}))
					assert.NoError(t, err)
					assert.Equal(t, role_base.Role_ROLE_UNSPECIFIED, res.Msg.Role)
				})
			})
		})
}
