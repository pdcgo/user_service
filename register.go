package user_service

import (
	"net/http"

	"connectrpc.com/connect"
	"github.com/pdcgo/san_collection/san_caches"
	"github.com/pdcgo/san_collection/san_verification"
	"github.com/pdcgo/schema/services/user_iface/v2/user_ifaceconnect"
	"github.com/pdcgo/shared/configs"
	"github.com/pdcgo/shared/custom_connect"
	"github.com/pdcgo/user_service/access_interceptors"
	"github.com/pdcgo/user_service/auth"
	"github.com/pdcgo/user_service/user"
	"gorm.io/gorm"
)

type ServiceReflectNames []string
type RegisterHandler func() ServiceReflectNames

func NewRegister(
	mux *http.ServeMux,
	db *gorm.DB,
	cfg *configs.AppConfig,
	defaultInterceptor custom_connect.DefaultInterceptor,
	cacheMgr san_caches.CacheManager,
) RegisterHandler {
	return func() ServiceReflectNames {
		grpcReflects := ServiceReflectNames{}

		authPath, authHandler := user_ifaceconnect.NewV2AuthServiceHandler(
			auth.NewV2AuthService(db, cfg.JwtSecret, cacheMgr),
			defaultInterceptor,
		)
		mux.Handle(authPath, authHandler)
		grpcReflects = append(grpcReflects, user_ifaceconnect.V2AuthServiceName)

		roleOpt := connect.WithInterceptors(access_interceptors.NewAccessInterceptor(db, cfg.JwtSecret, cacheMgr))
		userPath, userHandler := user_ifaceconnect.NewV2UserServiceHandler(
			user.NewV2UserService(db, san_verification.NewTwilioOtpVerification(&cfg.TwilioConfig)),
			defaultInterceptor,
			roleOpt,
		)
		mux.Handle(userPath, userHandler)
		grpcReflects = append(grpcReflects, user_ifaceconnect.V2UserServiceName)

		return grpcReflects
	}
}
