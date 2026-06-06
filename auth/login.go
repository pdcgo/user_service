package auth

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"github.com/pdcgo/schema/services/user_iface/v2"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/identity"
	"github.com/pdcgo/user_service/user_models"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// Login implements [user_ifaceconnect.V2AuthServiceHandler].
func (a *v2AuthServiceImpl) Login(
	ctx context.Context,
	req *connect.Request[user_iface.LoginRequest],
) (*connect.Response[user_iface.LoginResponse], error) {
	pay := req.Msg
	db := a.db.WithContext(ctx)

	q := db.Model(&user_models.User{})
	switch {
	case pay.GetEmail() != "":
		q = q.Where("email = ?", pay.GetEmail())
	case pay.GetUsername() != "":
		q = q.Where("username = ?", pay.GetUsername())
	case pay.GetPhone() != "":
		q = q.Where("phone_number = ?", pay.GetPhone())
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("email, username, or phone is required"))
	}

	var usr user_models.User
	if err := q.First(&usr).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
		}
		return nil, err
	}

	if usr.IsSuspended {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user is suspended"))
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usr.Password), []byte(pay.Password)); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
	}

	// Drop any stale cached roles so this session re-reads fresh authorization.
	_ = a.cacheMgr.DelNamespace(ctx, access_interceptors.UserRoleNamespace(usr.ID))

	now := time.Now()
	tok := &identity.TokenIdentity{
		Identity: &role_base.Identity{
			IdentityId:   uint32(usr.ID),
			IdentityType: pay.IndentityType,
			Username:     usr.Username,
			Agent:        pay.Agent,
			AgentVersion: pay.AgentVersion,
			ExpiredAt:    timestamppb.New(now.Add(24 * time.Hour)),
		},
	}
	token, err := tok.Serialize(a.secret)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&user_iface.LoginResponse{
		Token:    token,
		Identity: tok.Identity,
		User: &user_iface.User{
			Id:       uint64(usr.ID),
			Email:    usr.Email,
			Username: usr.Username,
			Name:     usr.Name,
			Status:   user_iface.UserStatus_USER_STATUS_ACTIVE,
		},
	}), nil
}
