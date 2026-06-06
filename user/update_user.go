package user

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/user_models"
	"golang.org/x/crypto/bcrypt"
)

// UpdateUser implements [user_ifaceconnect.V2UserServiceHandler].
//
// Only non-empty fields are updated; an empty password leaves the existing one
// intact.
func (s *v2UserServiceImpl) UpdateUser(
	ctx context.Context,
	req *connect.Request[user_iface.UpdateUserRequest],
) (*connect.Response[user_iface.UpdateUserResponse], error) {
	pay := req.Msg

	updates := map[string]interface{}{}
	if pay.Email != "" {
		updates["email"] = strings.ToLower(strings.TrimSpace(pay.Email))
	}
	if pay.Username != "" {
		updates["username"] = strings.ToLower(strings.TrimSpace(pay.Username))
	}
	if pay.Name != "" {
		updates["name"] = pay.Name
	}
	if pay.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(pay.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		updates["password"] = string(hash)
		updates["last_password_reset"] = time.Now()
	}

	db := s.db.WithContext(ctx)
	if len(updates) > 0 {
		if err := db.
			Model(&user_models.User{}).
			Where("id = ?", pay.Id).
			Updates(updates).
			Error; err != nil {
			return nil, err
		}
	}

	return connect.NewResponse(&user_iface.UpdateUserResponse{}), nil
}
