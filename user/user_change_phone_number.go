package user

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/user_models"
)

// UserChangePhoneNumber implements [user_ifaceconnect.V2UserServiceHandler]. It is
// a two-step, OTP-verified phone change: the `otp` action sends a one-time code to
// the new phone, and the `update` action verifies that code and persists the
// number. Authenticated callers only; user_id defaults to the caller, and a
// non-zero user_id targets that user instead.
func (s *v2UserServiceImpl) UserChangePhoneNumber(
	ctx context.Context,
	req *connect.Request[user_iface.UserChangePhoneNumberRequest],
) (*connect.Response[user_iface.UserChangePhoneNumberResponse], error) {
	caller, err := access_interceptors.GetIdentityFromCtx(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	userID := uint64(caller.IdentityId)
	if req.Msg.UserId != 0 {
		userID = req.Msg.UserId
	}

	switch {
	case req.Msg.GetOtp() != nil:
		// Send a one-time code to the new phone.
		phone := strings.TrimSpace(req.Msg.GetOtp().Phone)
		if phone == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("phone is required"))
		}
		if err := s.otp.Send(phone); err != nil {
			return nil, err
		}

	case req.Msg.GetUpdate() != nil:
		// Verify the code for the new phone, then persist it.
		u := req.Msg.GetUpdate()
		phone := strings.TrimSpace(u.Phone)
		if phone == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("phone is required"))
		}
		ok, err := s.otp.Verify(u.Otp, phone)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("invalid otp"))
		}
		if err := s.db.WithContext(ctx).
			Model(&user_models.User{}).
			Where("id = ?", userID).
			Update("phone_number", phone).
			Error; err != nil {
			return nil, err
		}

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("otp or update action is required"))
	}

	return connect.NewResponse(&user_iface.UserChangePhoneNumberResponse{}), nil
}
