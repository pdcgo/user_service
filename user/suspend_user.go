package user

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
)

// SuspendUser implements [user_ifaceconnect.V2UserServiceHandler]. It marks the
// user as suspended. Access is root/admin only (enforced by the access
// interceptor via the request_policy).
func (s *v2UserServiceImpl) SuspendUser(
	ctx context.Context,
	req *connect.Request[user_iface.SuspendUserRequest],
) (*connect.Response[user_iface.SuspendUserResponse], error) {
	db := s.db.WithContext(ctx)
	if err := db.
		Model(&user_models.User{}).
		Where("id = ?", req.Msg.UserId).
		Update("is_suspended", true).
		Error; err != nil {
		return nil, err
	}

	return connect.NewResponse(&user_iface.SuspendUserResponse{}), nil
}
