package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/san_collection/san_verification"
	"github.com/pdcgo/user_service/user"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestSuspendUser(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "suspend user",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}))

				u := &user_models.User{Username: "alice", Email: "alice@x.com", Name: "Alice"}
				assert.NoError(t, tx.Create(u).Error)

				svc := user.NewV2UserService(tx, san_verification.NewMockOtpVerification())
				ctx := context.Background()

				suspended := func() bool {
					var got user_models.User
					assert.NoError(t, tx.First(&got, u.ID).Error)
					return got.IsSuspended
				}

				t.Run("suspend marks the user suspended", func(t *testing.T) {
					_, err := svc.SuspendUser(ctx, connect.NewRequest(&user_iface.SuspendUserRequest{
						UserId: uint64(u.ID), Suspend: true,
					}))
					assert.NoError(t, err)
					assert.True(t, suspended())
				})

				t.Run("un-suspend clears it", func(t *testing.T) {
					_, err := svc.SuspendUser(ctx, connect.NewRequest(&user_iface.SuspendUserRequest{
						UserId: uint64(u.ID), Suspend: false,
					}))
					assert.NoError(t, err)
					assert.False(t, suspended())
				})
			})
		},
	)
}
