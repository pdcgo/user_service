package auth

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/identity"
)

// Logout implements [user_ifaceconnect.V2AuthServiceHandler].
//
// Tokens are stateless JWTs, so logout is a client-side concern; the server
// just evicts the caller's cached roles (best-effort) and acknowledges. Logout
// is public, so the token is read from the header here rather than the context.
func (a *v2AuthServiceImpl) Logout(
	ctx context.Context,
	req *connect.Request[user_iface.LogoutRequest],
) (*connect.Response[user_iface.LogoutResponse], error) {
	token := strings.TrimSpace(strings.TrimPrefix(req.Header().Get("Authorization"), "Bearer "))
	if token != "" {
		if tok, err := identity.Parse(a.secret, token); err == nil {
			_ = a.cacheMgr.DelNamespace(ctx, access_interceptors.UserRoleNamespace(uint(tok.IdentityId)))
		}
	}

	return connect.NewResponse(&user_iface.LogoutResponse{}), nil
}
