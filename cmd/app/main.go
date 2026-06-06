package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/pdcgo/shared/configs"
	"github.com/pdcgo/shared/custom_connect"
	"github.com/pdcgo/shared/db_connect"
	"github.com/pdcgo/user_service"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"gorm.io/gorm"
)

func NewDatabase(cfg *configs.AppConfig) (*gorm.DB, error) {
	return db_connect.NewProductionDatabase("user_service", &cfg.Database)
}

func NewRedisDatabase(cfg *configs.AppConfig) *redis.Client {
	return db_connect.NewRedisDatabase(&cfg.RedisConfig)
}

type App struct {
	Run func() error
}

func NewApp(
	mux *http.ServeMux,
	register user_service.RegisterHandler,
	reflectorRegister custom_connect.RegisterReflectFunc,
) *App {
	return &App{
		Run: func() error {
			reflectorRegister(register())

			port := os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}
			listen := fmt.Sprintf("%s:%s", os.Getenv("HOST"), port)
			log.Println("listening on", listen)

			return http.ListenAndServe(
				listen,
				// Use h2c so we can serve HTTP/2 without TLS.
				h2c.NewHandler(custom_connect.WithCORS(mux), &http2.Server{}),
			)
		},
	}
}

func main() {
	app, err := InitializeApp()
	if err != nil {
		panic(err)
	}

	if err := app.Run(); err != nil {
		panic(err)
	}
}
