package user_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/user_service/user"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestSearchUser(t *testing.T) {
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "search user",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}, &user_models.UserTeamRole{}))
				svc := user.NewV2UserService(tx)
				ctx := context.Background()

				u := &user_models.User{
					Username:       "carol",
					Email:          "carol@x.com",
					Name:           "Carol",
					ProfilePicture: "pic-carol",
				}
				assert.NoError(t, tx.Create(u).Error)

				t.Run("by ids returns profile_picture", func(t *testing.T) {
					res, err := svc.SearchUser(ctx, connect.NewRequest(&user_iface.SearchUserRequest{
						Filter: &user_iface.SearchUserRequest_Ids{
							Ids: &user_iface.SearchUserByIds{Ids: []uint64{uint64(u.ID)}},
						},
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, uint64(u.ID), res.Msg.Users[0].Id)
					assert.Equal(t, "pic-carol", res.Msg.Users[0].ProfilePicture)
				})

				t.Run("by keyword returns profile_picture", func(t *testing.T) {
					res, err := svc.SearchUser(ctx, connect.NewRequest(&user_iface.SearchUserRequest{
						Filter: &user_iface.SearchUserRequest_Keyword{
							Keyword: &user_iface.SearchUserByKeyword{Q: "carol"},
						},
					}))
					assert.NoError(t, err)
					assert.Len(t, res.Msg.Users, 1)
					assert.Equal(t, "pic-carol", res.Msg.Users[0].ProfilePicture)
				})
			})
		})
}
