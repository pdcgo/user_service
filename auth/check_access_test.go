package auth_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_caches"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/auth"
	"github.com/pdcgo/user_service/identity"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func makeToken(t *testing.T, secret string, expiredAt time.Time) string {
	t.Helper()
	tok := &identity.TokenIdentity{
		Identity: &role_base.Identity{
			IdentityId:   42,
			IdentityType: role_base.IdentityType_IDENTITY_TYPE_GENERAL_USER,
			Username:     "alice",
			ExpiredAt:    timestamppb.New(expiredAt),
		},
	}
	s, err := tok.Serialize(secret)
	assert.NoError(t, err)
	return s
}

func TestCheckAccess(t *testing.T) {
	const secret = "test-secret"
	svc := auth.NewV2AuthService(nil, secret, san_caches.NewSkipCacheManager())
	ctx := context.Background()

	t.Run("valid token returned unchanged", func(t *testing.T) {
		token := makeToken(t, secret, time.Now().Add(time.Hour))
		res, err := svc.CheckAccess(ctx,
			connect.NewRequest(&user_iface.CheckAccessRequest{Token: token}))
		assert.NoError(t, err)
		assert.Equal(t, token, res.Msg.Token)
	})

	t.Run("expired token is refreshed", func(t *testing.T) {
		token := makeToken(t, secret, time.Now().Add(-time.Hour))
		res, err := svc.CheckAccess(ctx,
			connect.NewRequest(&user_iface.CheckAccessRequest{Token: token}))
		assert.NoError(t, err)
		assert.NotEqual(t, token, res.Msg.Token)

		refreshed, err := identity.Parse(secret, res.Msg.Token)
		assert.NoError(t, err)
		assert.False(t, refreshed.IsExpired(time.Now()))
		assert.Equal(t, uint32(42), refreshed.IdentityId)
		assert.Equal(t, "alice", refreshed.Username)
	})

	t.Run("garbage token is unauthenticated", func(t *testing.T) {
		_, err := svc.CheckAccess(ctx,
			connect.NewRequest(&user_iface.CheckAccessRequest{Token: "not-a-token"}))
		assert.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("token signed with wrong secret is unauthenticated", func(t *testing.T) {
		token := makeToken(t, "other-secret", time.Now().Add(time.Hour))
		_, err := svc.CheckAccess(ctx,
			connect.NewRequest(&user_iface.CheckAccessRequest{Token: token}))
		assert.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})
}
