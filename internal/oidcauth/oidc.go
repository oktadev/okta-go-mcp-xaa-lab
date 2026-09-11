package oidcauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"

	"oktane-go-mcp-xaa/internal/config"
	"oktane-go-mcp-xaa/internal/session"
)

const flowCookieName = "oidc_flow"

// Auth drives the OIDC + PKCE login flow: /login, /callback, /logout.
// ASP.NET Core's OIDC middleware persists state/verifier/nonce in
// AuthenticationProperties; Go has no equivalent, so we stash them in a
// short-lived (5 min), HttpOnly cookie cleared on callback.
type Auth struct {
	cfg         config.XaaConfig
	provider    *oidc.Provider
	oauth2Cfg   oauth2.Config
	verifier    *oidc.IDTokenVerifier
	flowCookies *sessions.CookieStore
	sess        *session.Store
}

func New(ctx context.Context, cfg config.XaaConfig, sess *session.Store) (*Auth, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IdpBaseURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery at %s: %w", cfg.IdpBaseURL, err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	flowCookies := sessions.NewCookieStore([]byte(cfg.SessionSecret))
	flowCookies.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	return &Auth{
		cfg:         cfg,
		provider:    provider,
		oauth2Cfg:   oauth2Cfg,
		verifier:    provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		flowCookies: flowCookies,
		sess:        sess,
	}, nil
}

// HandleLogin signs out any existing session before challenging — lets a
// different user authenticate, mirroring the C#/TS ports.
func (a *Auth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if d, ok := a.sess.Get(r); ok && d.Authenticated {
		_ = a.sess.Clear(w, r)
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	verifier := oauth2.GenerateVerifier()
	state := randString()
	nonce := randString()

	flow, _ := a.flowCookies.Get(r, flowCookieName)
	flow.Values["state"] = state
	flow.Values["verifier"] = verifier
	flow.Values["nonce"] = nonce
	if err := flow.Save(r, w); err != nil {
		http.Error(w, "failed to start login", http.StatusInternalServerError)
		return
	}

	authURL := a.oauth2Cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (a *Auth) HandleCallback(w http.ResponseWriter, r *http.Request) {
	flow, _ := a.flowCookies.Get(r, flowCookieName)
	wantState, _ := flow.Values["state"].(string)
	verifier, _ := flow.Values["verifier"].(string)
	nonce, _ := flow.Values["nonce"].(string)
	flow.Options.MaxAge = -1
	_ = flow.Save(r, w)

	if wantState == "" || r.URL.Query().Get("state") != wantState {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	ctx := r.Context()
	tok, err := a.oauth2Cfg.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil || idToken.Nonce != nonce {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	_ = idToken.Claims(&claims)

	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}
	if email == "" {
		email = idToken.Subject
	}
	if email == "" {
		email = "unknown"
	}

	if err := a.sess.Save(w, r, session.Data{
		Authenticated: true,
		Email:         email,
		IDToken:       rawIDToken,
		AuthMethod:    "oidc",
	}); err != nil {
		http.Error(w, "failed to save session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?autorun=true", http.StatusFound)
}

// HandleLogout clears the local session and, if the IdP advertises an
// end_session_endpoint, performs RP-initiated logout there too.
func (a *Auth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	_ = a.sess.Clear(w, r)

	var discoveryClaims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := a.provider.Claims(&discoveryClaims); err == nil && discoveryClaims.EndSessionEndpoint != "" {
		if u, err := url.Parse(discoveryClaims.EndSessionEndpoint); err == nil {
			q := u.Query()
			q.Set("post_logout_redirect_uri", postLogoutRedirectURI(a.cfg.RedirectURI))
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLoggedOut is the target of the post-logout redirect.
func (a *Auth) HandleLoggedOut(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

func postLogoutRedirectURI(redirectURI string) string {
	return strings.TrimSuffix(redirectURI, "/callback") + "/logged-out"
}

func randString() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
