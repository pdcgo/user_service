package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/pdcgo/shared/custom_connect"
	"github.com/pdcgo/user_service"
	"github.com/urfave/cli/v3"
)

type ServiceApiFunc cli.ActionFunc

func NewServiceApiFunc(
	mux *http.ServeMux,
	register user_service.RegisterHandler,
	reflectorRegister custom_connect.RegisterReflectFunc,
) ServiceApiFunc {
	return func(ctx context.Context, c *cli.Command) error {
		cancel, err := custom_connect.InitTracer("user-service")
		if err != nil {
			return err
		}
		defer cancel(context.Background())

		reflectorRegister(register())

		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		listen := fmt.Sprintf("%s:%s", os.Getenv("HOST"), port)
		log.Println("listening on", listen)

		// Serve HTTP/1.1 and unencrypted HTTP/2 (h2c) without TLS. Native h2c via
		// Server.Protocols (Go 1.24+) replaces the deprecated
		// golang.org/x/net/http2/h2c.NewHandler.
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetUnencryptedHTTP2(true)

		srv := &http.Server{
			Addr:      listen,
			Handler:   custom_connect.WithCORS(mux),
			Protocols: protocols,
		}
		return srv.ListenAndServe()
	}
}
