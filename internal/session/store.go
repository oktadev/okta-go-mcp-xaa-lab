package session

import (
	"net/http"

	"github.com/gorilla/sessions"
)

const cookieName = "xaa.auth"

// Data is the authenticated-user state kept in the session cookie.
type Data struct {
	Authenticated bool
	Email         string
	IDToken       string // only ever set for AuthMethod "oidc" — SAML has no id_token
	AuthMethod    string // "oidc" | "saml"
}

// Store wraps a gorilla/sessions cookie store, matching the C# app's
// non-persistent, 1-hour cookie session (`xaa.auth`, ExpireTimeSpan=1h).
type Store struct {
	cookies *sessions.CookieStore
}

func New(secret string) *Store {
	cs := sessions.NewCookieStore([]byte(secret))
	cs.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	return &Store{cookies: cs}
}

func (s *Store) Get(r *http.Request) (Data, bool) {
	sess, err := s.cookies.Get(r, cookieName)
	if err != nil {
		return Data{}, false
	}
	authenticated, _ := sess.Values["authenticated"].(bool)
	if !authenticated {
		return Data{}, false
	}
	return Data{
		Authenticated: true,
		Email:         asString(sess.Values["email"]),
		IDToken:       asString(sess.Values["idToken"]),
		AuthMethod:    asString(sess.Values["authMethod"]),
	}, true
}

func (s *Store) Save(w http.ResponseWriter, r *http.Request, d Data) error {
	sess, _ := s.cookies.Get(r, cookieName)
	sess.Values["authenticated"] = d.Authenticated
	sess.Values["email"] = d.Email
	sess.Values["idToken"] = d.IDToken
	sess.Values["authMethod"] = d.AuthMethod
	return sess.Save(r, w)
}

func (s *Store) Clear(w http.ResponseWriter, r *http.Request) error {
	sess, _ := s.cookies.Get(r, cookieName)
	sess.Options.MaxAge = -1
	return sess.Save(r, w)
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}
