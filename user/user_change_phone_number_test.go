package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_verification"
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

func TestUserChangePhoneNumber(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "user change phone number",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}))

				alice := &user_models.User{Username: "alice", Email: "alice@x.com", Name: "Alice", PhoneNumber: "+62800old"}
				assert.NoError(t, tx.Create(alice).Error)

				// the mock OTP accepts san_verification.MockOtpCode and rejects anything else.
				svc := user.NewV2UserService(tx, san_verification.NewMockOtpVerification())
				aliceCtx := access_interceptors.SetIdentityToCtx(
					context.Background(),
					&role_base.Identity{IdentityId: uint32(alice.ID)},
				)

				phoneOf := func() string {
					var got user_models.User
					assert.NoError(t, tx.First(&got, alice.ID).Error)
					return got.PhoneNumber
				}

				t.Run("unauthenticated caller is rejected", func(t *testing.T) {
					_, err := svc.UserChangePhoneNumber(context.Background(), connect.NewRequest(&user_iface.UserChangePhoneNumberRequest{
						Action: &user_iface.UserChangePhoneNumberRequest_Otp{
							Otp: &user_iface.UserChangePhoneNumberSendOtp{Phone: "+62811"},
						},
					}))
					assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
				})

				t.Run("otp action sends without error", func(t *testing.T) {
					_, err := svc.UserChangePhoneNumber(aliceCtx, connect.NewRequest(&user_iface.UserChangePhoneNumberRequest{
						Action: &user_iface.UserChangePhoneNumberRequest_Otp{
							Otp: &user_iface.UserChangePhoneNumberSendOtp{Phone: "+62811new"},
						},
					}))
					assert.NoError(t, err)
					// sending an OTP does not change the stored number yet
					assert.Equal(t, "+62800old", phoneOf())
				})

				t.Run("update with valid otp persists the new number", func(t *testing.T) {
					_, err := svc.UserChangePhoneNumber(aliceCtx, connect.NewRequest(&user_iface.UserChangePhoneNumberRequest{
						Action: &user_iface.UserChangePhoneNumberRequest_Update{
							Update: &user_iface.UserChangePhoneNumberSendUpdate{Phone: "+62811new", Otp: san_verification.MockOtpCode},
						},
					}))
					assert.NoError(t, err)
					assert.Equal(t, "+62811new", phoneOf())
				})

				t.Run("update with wrong otp is rejected and leaves the number unchanged", func(t *testing.T) {
					_, err := svc.UserChangePhoneNumber(aliceCtx, connect.NewRequest(&user_iface.UserChangePhoneNumberRequest{
						Action: &user_iface.UserChangePhoneNumberRequest_Update{
							Update: &user_iface.UserChangePhoneNumberSendUpdate{Phone: "+62899evil", Otp: "000000"},
						},
					}))
					assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
					assert.Equal(t, "+62811new", phoneOf())
				})

				t.Run("empty action is invalid argument", func(t *testing.T) {
					_, err := svc.UserChangePhoneNumber(aliceCtx, connect.NewRequest(&user_iface.UserChangePhoneNumberRequest{}))
					assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
				})
			})
		})
}
