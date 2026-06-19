package auth

import (
	"context"
	"time"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/identity"
	"github.com/pdcgo/user_service/user_models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CheckAccess implements [user_ifaceconnect.V2AuthServiceHandler]. It verifies
// the token signature and, if the token is expired, returns a fresh token with
// extended expiry; a still-valid token is returned unchanged. When a team_id is
// supplied, it also returns the caller's role in that team (ROLE_UNSPECIFIED if
// the caller is not a member).
func (a *v2AuthServiceImpl) CheckAccess(
	ctx context.Context,
	req *connect.Request[user_iface.CheckAccessRequest],
) (*connect.Response[user_iface.CheckAccessResponse], error) {
	tok, err := identity.Parse(a.secret, req.Msg.Token)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	now := time.Now()
	token := req.Msg.Token
	if tok.IsExpired(now) {
		tok.ExpiredAt = timestamppb.New(now.Add(24 * time.Hour))
		token, err = tok.Serialize(a.secret)
		if err != nil {
			return nil, err
		}
	}

	resp := &user_iface.CheckAccessResponse{Token: token}

	// When a team is specified, surface the caller's role in that team.
	if req.Msg.TeamId != 0 {
		var utr user_models.UserTeamRole
		if err := a.db.WithContext(ctx).
			Where("user_id = ? AND team_id = ?", uint(tok.IdentityId), req.Msg.TeamId).
			Limit(1).
			Find(&utr).Error; err != nil {
			return nil, err
		}
		resp.Role = role_base.Role(utr.Role) // non-member -> ROLE_UNSPECIFIED (0)
	}

	return connect.NewResponse(resp), nil
}
