package user

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
)

// DeleteUser implements [user_ifaceconnect.V2UserServiceHandler].
//
// user_models.User has no soft-delete field, so this is a hard delete.
func (s *v2UserServiceImpl) DeleteUser(
	ctx context.Context,
	req *connect.Request[user_iface.DeleteUserRequest],
) (*connect.Response[user_iface.DeleteUserResponse], error) {
	pay := req.Msg

	db := s.db.WithContext(ctx)
	if err := db.Delete(&user_models.User{}, pay.Id).Error; err != nil {
		return nil, err
	}

	return connect.NewResponse(&user_iface.DeleteUserResponse{}), nil
}
