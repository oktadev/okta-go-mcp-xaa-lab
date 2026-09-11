package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"oktane-go-mcp-xaa/internal/api"
	"oktane-go-mcp-xaa/internal/config"
	"oktane-go-mcp-xaa/internal/oidcauth"
	"oktane-go-mcp-xaa/internal/samlauth"
	"oktane-go-mcp-xaa/internal/session"
)

func main() {
	cfgPath := "config.json"
	if v := os.Getenv("CONFIG_PATH"); v != "" {
		cfgPath = v
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	sess := session.New(cfg.Xaa.SessionSecret)
	ctx := context.Background()

	oidcAuth, err := oidcauth.New(ctx, cfg.Xaa, sess)
	if err != nil {
		log.Fatalf("init oidc: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	if cfg.Saml.Enabled {
		samlAuth, err := samlauth.New(ctx, cfg.Saml, sess)
		if err != nil {
			log.Printf("SAML login disabled — setup failed: %v", err)
		} else {
			r.Mount("/saml/", samlAuth.Middleware())
			r.Get("/login/saml", samlAuth.HandleLogin)
			r.Get("/logout/saml", samlAuth.HandleLogout)
			log.Println("SAML SSO enabled at /login/saml")
		}
	}

	r.Get("/login", oidcAuth.HandleLogin)
	r.Get("/callback", oidcAuth.HandleCallback)
	r.Get("/logout", oidcAuth.HandleLogout)
	r.Get("/logged-out", oidcAuth.HandleLoggedOut)

	r.Get("/api/user", api.UserHandler(sess))
	r.Get("/api/flow", api.FlowHandler(cfg.Xaa, sess))

	r.Handle("/*", http.FileServer(http.Dir("web")))

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("AI Notes Assistant listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
