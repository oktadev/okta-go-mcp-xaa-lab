package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"oktane-go-mcp-xaa/internal/config"
	"oktane-go-mcp-xaa/internal/mcpnotes"
	"oktane-go-mcp-xaa/internal/session"
	"oktane-go-mcp-xaa/internal/xaa"
)

type userResponse struct {
	LoggedIn bool   `json:"loggedIn"`
	Email    string `json:"email,omitempty"`
}

func UserHandler(sess *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		d, ok := sess.Get(r)
		if !ok || !d.Authenticated {
			_ = json.NewEncoder(w).Encode(userResponse{LoggedIn: false})
			return
		}
		_ = json.NewEncoder(w).Encode(userResponse{LoggedIn: true, Email: d.Email})
	}
}

// FlowHandler streams the XAA flow over SSE. Event names/payload shapes match
// the C#/TS ports exactly (step, detail, success "msg|ms", token, claims,
// data, error, done), so the shared web/index.html JS needs no changes.
func FlowHandler(cfg config.XaaConfig, sess *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := sess.Get(r)
		if !ok || !d.Authenticated {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, _ := w.(http.Flusher)

		send := func(evt, data string) {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt, data)
			if flusher != nil {
				flusher.Flush()
			}
		}

		ctx := r.Context()

		// XAA's RFC 8693 token exchange requires an OIDC id_token as subject_token.
		// SAML assertions don't produce one, so a SAML-authenticated session can't
		// run this flow — that's a protocol constraint, not a bug.
		if d.AuthMethod != "oidc" {
			send("step", "Step 1 — XAA Auth Flow")
			send("error", "The XAA token-exchange flow requires an OIDC id_token — please sign in via Enterprise SSO (not SAML) to run this demo.")
			send("done", "failed")
			return
		}

		start := time.Now()
		send("step", "Step 1 — User Login (OIDC)")
		send("detail", fmt.Sprintf("OIDC middleware completed login at %s", cfg.IdpBaseURL))
		send("detail", "PKCE code exchange → ID Token stored in server-side session")
		send("success", "ID Token obtained from session|0")

		step2Start := time.Now()
		send("step", "Step 2 — Token Exchange (RFC 8693)")
		send("detail", fmt.Sprintf("POST %s/token", cfg.IdpBaseURL))
		send("detail", "grant_type=token-exchange · subject_token=<id_token>")
		send("detail", fmt.Sprintf("audience=%s · resource=%s", cfg.AuthServerURL, cfg.McpServerURL))

		accessToken, err := xaa.GetAccessToken(ctx, cfg, d.IDToken)
		if err != nil {
			send("error", err.Error())
			send("done", "failed")
			return
		}
		send("success", fmt.Sprintf("ID-JAG assertion issued by IdP|%d", time.Since(step2Start).Milliseconds()))

		step3Start := time.Now()
		send("step", "Step 3 — Access Token Request (RFC 7523)")
		send("detail", fmt.Sprintf("POST %s/token", cfg.AuthServerURL))
		send("detail", fmt.Sprintf("grant_type=jwt-bearer · assertion=<ID-JAG> · scope=%s", cfg.Scope))
		send("success", fmt.Sprintf("Bearer token issued (aud: %s)|%d", cfg.McpServerURL, time.Since(step3Start).Milliseconds()))

		tokenPreview := accessToken
		if len(tokenPreview) > 50 {
			tokenPreview = tokenPreview[:50]
		}
		send("token", tokenPreview)

		if claims, ok := decodeClaims(accessToken); ok {
			b, _ := json.Marshal(claims)
			send("claims", string(b))
		}

		step4Start := time.Now()
		send("step", "Step 4 — MCP Server (Streamable HTTP)")
		send("detail", fmt.Sprintf("StreamableHTTPClientTransport → %s", cfg.McpServerURL))
		send("detail", fmt.Sprintf("Authorization: Bearer <token> · ReadResource: %s", cfg.ResourceURI))

		result, err := mcpnotes.Fetch(ctx, cfg, accessToken)
		if err != nil {
			send("error", err.Error())
			send("done", "failed")
			return
		}
		step4Elapsed := time.Since(step4Start).Milliseconds()
		totalMs := time.Since(start).Milliseconds()

		send("detail", fmt.Sprintf("Resources: %s", strings.Join(result.Resources, ", ")))
		send("success", fmt.Sprintf("%d note(s) from MCP Server|%d", len(result.Notes), step4Elapsed))

		rawPreview := result.RawContent
		if len(rawPreview) > 300 {
			rawPreview = rawPreview[:300] + "…"
		}

		payload := map[string]any{
			"resources":  result.Resources,
			"notes":      result.Notes,
			"rawContent": rawPreview,
			"insights":   BuildInsights(result.Notes),
			"totalMs":    totalMs,
		}
		b, _ := json.Marshal(payload)
		send("data", string(b))
		send("done", "ok")
	}
}

func decodeClaims(token string) (map[string]string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}

	var full map[string]any
	if err := json.Unmarshal(payload, &full); err != nil {
		return nil, false
	}

	out := map[string]string{}
	for _, k := range []string{"iss", "aud", "sub"} {
		if v, ok := full[k]; ok {
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out, true
}
