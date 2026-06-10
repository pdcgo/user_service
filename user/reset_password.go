package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ResetPassword implements [user_ifaceconnect.V2UserServiceHandler].
//
// Self-service change-password: the caller proves ownership by supplying the
// current (old) password, then sets a new one. The RPC is public at the
// interceptor level (allow_all); the old-password check is the real gate.
func (s *v2UserServiceImpl) ResetPassword(
	ctx context.Context,
	req *connect.Request[user_iface.ResetPasswordRequest],
) (*connect.Response[user_iface.ResetPasswordResponse], error) {
	pay := req.Msg

	if strings.TrimSpace(pay.NewPassword) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new password is required"))
	}

	db := s.db.WithContext(ctx)

	var usr user_models.User
	if err := db.Where("id = ?", pay.UserId).First(&usr).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usr.Password), []byte(pay.OldPassword)); err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("old password incorrect"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pay.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	if err := db.
		Model(&user_models.User{}).
		Where("id = ?", pay.UserId).
		Updates(map[string]interface{}{
			"password":            string(hash),
			"last_password_reset": time.Now(),
		}).
		Error; err != nil {
		return nil, err
	}

	return connect.NewResponse(&user_iface.ResetPasswordResponse{}), nil
}
