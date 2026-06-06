package auth

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/identity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CheckAccess implements [user_ifaceconnect.V2AuthServiceHandler]. It verifies
// the token signature and, if the token is expired, returns a fresh token with
// extended expiry; a still-valid token is returned unchanged.
func (a *v2AuthServiceImpl) CheckAccess(
	ctx context.Context,
	req *connect.Request[user_iface.CheckAccessRequest],
) (*connect.Response[user_iface.CheckAccessResponse], error) {
	tok, err := identity.Parse(a.secret, req.Msg.Token)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	now := time.Now()
	if !tok.IsExpired(now) {
		return connect.NewResponse(&user_iface.CheckAccessResponse{Token: req.Msg.Token}), nil
	}

	tok.ExpiredAt = timestamppb.New(now.Add(24 * time.Hour))
	refreshed, err := tok.Serialize(a.secret)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&user_iface.CheckAccessResponse{Token: refreshed}), nil
}
