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

// CreateUser implements [user_ifaceconnect.V2UserServiceHandler].
func (s *v2UserServiceImpl) CreateUser(
	ctx context.Context,
	req *connect.Request[user_iface.CreateUserRequest],
) (*connect.Response[user_iface.CreateUserResponse], error) {
	pay := req.Msg

	hash, err := bcrypt.GenerateFromPassword([]byte(pay.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	usr := &user_models.User{
		Name:              pay.Name,
		Username:          strings.ToLower(strings.TrimSpace(pay.Username)),
		Email:             strings.ToLower(strings.TrimSpace(pay.Email)),
		Password:          string(hash),
		PhoneNumber:       strings.TrimSpace(pay.PhoneNumber),
		CreatedAt:         now,
		LastPasswordReset: now,
	}

	db := s.db.WithContext(ctx)
	if err := db.Create(usr).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("username or email already exists"))
		}
		return nil, err
	}

	return connect.NewResponse(&user_iface.CreateUserResponse{
		Id: uint64(usr.ID),
	}), nil
}
