package samlauth

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crewjam/saml/samlsp"

	"oktane-go-mcp-xaa/internal/config"
	"oktane-go-mcp-xaa/internal/session"
)

// Auth stands the app up as a SAML Service Provider — an alternate SSO
// front door alongside OIDC. IMPORTANT: XAA's RFC 8693 token exchange
// requires an OIDC id_token as subject_token; SAML assertions don't
// produce one, so a SAML-authenticated session can log a user into the
// app but cannot run the XAA/MCP "Analyze" flow (see api.FlowHandler).
type Auth struct {
	mw   *samlsp.Middleware
	sess *session.Store
}

func New(ctx context.Context, cfg config.SamlConfig, sess *session.Store) (*Auth, error) {
	keyPair, err := tls.LoadX509KeyPair(cfg.SPCertFile, cfg.SPKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading SP key pair: %w", err)
	}
	keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parsing SP certificate: %w", err)
	}

	rootURL, err := url.Parse(cfg.SPRootURL)
	if err != nil {
		return nil, fmt.Errorf("parsing spRootUrl: %w", err)
	}

	idpMetadataURL, err := url.Parse(cfg.IdpMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("parsing idpMetadataUrl: %w", err)
	}

	idpMetadata, err := samlsp.FetchMetadata(ctx, http.DefaultClient, *idpMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("fetching IdP metadata: %w", err)
	}

	mw, err := samlsp.New(samlsp.Options{
		URL:         *rootURL,
		Key:         keyPair.PrivateKey.(*rsa.PrivateKey),
		Certificate: keyPair.Leaf,
		IDPMetadata: idpMetadata,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing SAML SP: %w", err)
	}

	return &Auth{mw: mw, sess: sess}, nil
}

// Middleware serves the SP's internal endpoints (ACS, metadata, SLO) —
// mount it at "/saml/".
func (a *Auth) Middleware() *samlsp.Middleware {
	return a.mw
}

// HandleLogin requires a SAML session (redirecting to the IdP if absent),
// then bridges the resulting SAML attributes into our own app session.
func (a *Auth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	a.mw.RequireAccount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := "unknown"
		if sess, ok := samlsp.SessionFromContext(r.Context()).(samlsp.SessionWithAttributes); ok {
			attrs := sess.GetAttributes()
			for _, key := range []string{"email", "urn:oid:0.9.2342.19200300.100.1.3", "name", "urn:oid:2.5.4.3"} {
				if v := attrs.Get(key); v != "" {
					email = v
					break
				}
			}
		}

		if err := a.sess.Save(w, r, session.Data{
			Authenticated: true,
			Email:         email,
			AuthMethod:    "saml",
		}); err != nil {
			http.Error(w, "failed to save session", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/?autorun=true", http.StatusFound)
	})).ServeHTTP(w, r)
}

func (a *Auth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	_ = a.sess.Clear(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}
