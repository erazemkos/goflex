package main

import (
	"context"
	"io/fs"
	"log"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/erazemkos/goflex/examples/todo/internal/api"
	"github.com/erazemkos/goflex/examples/todo/internal/models"
	"github.com/erazemkos/goflex/pkg/auth"
	"github.com/erazemkos/goflex/pkg/db"
	"github.com/erazemkos/goflex/pkg/server"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "todo.db"
	}
	gdb := db.MustOpen(db.Config{Driver: "sqlite", DSN: dsn, Env: os.Getenv("GOFLEX_ENV")})
	store := models.NewStore(gdb)
	if os.Getenv("GOFLEX_ENV") != "prod" {
		if err := store.AutoMigrate(); err != nil {
			log.Fatal(err)
		}
	}
	authn := auth.NewAuth(auth.Config{
		Env:       os.Getenv("GOFLEX_ENV"),
		SecretKey: []byte(os.Getenv("SECRET_KEY")),
		UserLoader: func(ctx context.Context, id string) (auth.User, error) {
			uid, err := strconv.ParseUint(id, 10, 64)
			if err != nil {
				return auth.User{}, err
			}
			u, err := store.UserByID(ctx, uint(uid))
			return auth.User{ID: strconv.FormatUint(uint64(u.ID), 10), Email: u.Email, Name: u.Name}, err
		},
	})
	app := server.New(server.Config{Env: os.Getenv("GOFLEX_ENV"), StaticFS: staticFS()})
	app.Use(authn.Middleware(), authn.CSRFMiddleware())
	app.API("", func(r *gin.RouterGroup) { api.RegisterRoutes(r, authn, store) })
	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	log.Fatal(app.Run(addr))
}

func staticFS() fs.FS {
	if _, err := os.Stat("dist"); err == nil {
		return os.DirFS("dist")
	}
	return os.DirFS(".")
}
