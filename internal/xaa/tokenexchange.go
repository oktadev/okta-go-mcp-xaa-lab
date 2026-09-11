// Package xaa implements the Cross-App Access token chain: RFC 8693 token
// exchange (id_token -> ID-JAG) followed by an RFC 7523 JWT-bearer grant
// (ID-JAG -> access token). Ported field-for-field from the C# app's
// Services/SdkStub.cs (and mirrored in the TypeScript port's xaaService.ts),
// including the TrimRight('/') fix: xaa.dev's IdP/AuthServer do exact-string
// matching on audience/resource, so a bare-hostname URL must be trimmed.
package xaa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"oktane-go-mcp-xaa/internal/config"
)

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func postForm(ctx context.Context, endpoint string, fields url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(fields.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed tokenResponse
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || parsed.Error != "" {
		code := parsed.Error
		if code == "" {
			code = resp.Status
		}
		desc := parsed.ErrorDescription
		if desc == "" {
			desc = string(body)
		}
		return "", fmt.Errorf("token request to %s failed with status %d. Error: %s (%s)", endpoint, resp.StatusCode, code, desc)
	}

	return parsed.AccessToken, nil
}

// exchangeForJAG performs the RFC 8693 token exchange at the IdP.
func exchangeForJAG(ctx context.Context, cfg config.XaaConfig, idToken string) (string, error) {
	fields := url.Values{
		"grant_type":           {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"requested_token_type": {"urn:ietf:params:oauth:token-type:id-jag"},
		"subject_token":        {idToken},
		"subject_token_type":   {"urn:ietf:params:oauth:token-type:id_token"},
		"audience":             {strings.TrimRight(cfg.AuthServerURL, "/")},
		"resource":             {strings.TrimRight(cfg.McpServerURL, "/")},
		"client_id":            {cfg.ClientID},
	}
	if cfg.Scope != "" {
		fields.Set("scope", cfg.Scope)
	}
	if cfg.ClientSecret != "" {
		fields.Set("client_secret", cfg.ClientSecret)
	}

	return postForm(ctx, strings.TrimRight(cfg.IdpBaseURL, "/")+"/token", fields)
}

// requestAccessToken performs the RFC 7523 JWT-bearer grant at the resource auth server.
func requestAccessToken(ctx context.Context, cfg config.XaaConfig, jag string) (string, error) {
	fields := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jag},
		"client_id":  {cfg.McpClientID},
	}
	if cfg.Scope != "" {
		fields.Set("scope", cfg.Scope)
	}
	if cfg.McpClientSecret != "" {
		fields.Set("client_secret", cfg.McpClientSecret)
	}

	endpoint := strings.TrimRight(cfg.AuthServerURL, "/") + "/token"
	return postForm(ctx, endpoint, fields)
}

// GetAccessToken runs the full XAA exchange: id_token -> ID-JAG -> access token.
func GetAccessToken(ctx context.Context, cfg config.XaaConfig, idToken string) (string, error) {
	jag, err := exchangeForJAG(ctx, cfg, idToken)
	if err != nil {
		return "", err
	}
	return requestAccessToken(ctx, cfg, jag)
}
