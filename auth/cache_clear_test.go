package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_caches"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/shared/pkg/moretest"
	"github.com/pdcgo/shared/pkg/moretest/moretest_mock"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/auth"
	"github.com/pdcgo/user_service/user_models"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// spyCache records DelNamespace calls; Get always misses.
type spyCache struct {
	namespaces []string
}

func (s *spyCache) Set(ctx context.Context, key san_caches.CacheKey, value any, ttl time.Duration) error {
	return nil
}
func (s *spyCache) Get(ctx context.Context, key san_caches.CacheKey, value any) error {
	return errors.New("miss")
}
func (s *spyCache) Del(ctx context.Context, key san_caches.CacheKey) error { return nil }
func (s *spyCache) DelNamespace(ctx context.Context, namespace string) error {
	s.namespaces = append(s.namespaces, namespace)
	return nil
}

func TestLogoutClearsRoleCache(t *testing.T) {
	const secret = "test-secret"
	spy := &spyCache{}
	svc := auth.NewV2AuthService(nil, secret, spy)

	req := connect.NewRequest(&user_iface.LogoutRequest{})
	req.Header().Set("Authorization", "Bearer "+makeToken(t, secret, time.Now().Add(time.Hour)))

	_, err := svc.Logout(context.Background(), req)
	assert.NoError(t, err)
	// makeToken issues IdentityId = 42
	assert.Contains(t, spy.namespaces, access_interceptors.UserRoleNamespace(42))
}

func TestLoginClearsRoleCache(t *testing.T) {
	const secret = "test-secret"
	var scenario moretest_mock.DbScenario
	moretest.Suite(t, "login clears role cache",
		moretest.SetupListFunc{moretest_mock.MockPostgresDatabase(&scenario)},
		func(t *testing.T) {
			scenario(t, func(tx *gorm.DB) {
				assert.NoError(t, tx.AutoMigrate(&user_models.User{}))
				hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
				assert.NoError(t, err)
				usr := &user_models.User{Username: "alice", Email: "alice@x.com", Password: string(hash)}
				assert.NoError(t, tx.Create(usr).Error)

				spy := &spyCache{}
				svc := auth.NewV2AuthService(tx, secret, spy)

				_, err = svc.Login(context.Background(), connect.NewRequest(&user_iface.LoginRequest{
					Auth:     &user_iface.LoginRequest_Username{Username: "alice"},
					Password: "secret123",
				}))
				assert.NoError(t, err)
				assert.Contains(t, spy.namespaces, access_interceptors.UserRoleNamespace(usr.ID))
			})
		})
}
