//go:build wireinject
// +build wireinject

package main

import (
	"net/http"

	"github.com/google/wire"
	"github.com/pdcgo/san_collection/san_caches"
	"github.com/pdcgo/shared/configs"
	"github.com/pdcgo/shared/custom_connect"
	"github.com/pdcgo/user_service"
	"github.com/urfave/cli/v3"
)

func InitializeApp() (*cli.Command, error) {
	wire.Build(
		http.NewServeMux,
		configs.NewProductionConfig,
		NewDatabase,
		NewRedisDatabase,
		san_caches.NewRedisCacheManager,
		custom_connect.NewDefaultInterceptor,
		custom_connect.NewRegisterReflect,
		user_service.NewRegister,
		NewServiceApiFunc,
		NewSyncLegacyFunc,
		NewApp,
	)
	return &cli.Command{}, nil
}
