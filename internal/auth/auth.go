// Package auth handles logging in to GitHub two ways:
//
//   - PAT: user pastes a Personal Access Token. Works immediately, no
//     OAuth app registration required. Good for getting started today.
//   - Device flow: gh-style. We print a code, the user approves in the
//     browser, and we poll for the token. No secret copy-pasting. This
//     is the "install and just use it" experience — but it needs a
//     registered OAuth App Client ID (set via LAZYHUB_CLIENT_ID or the
//     baked-in default below).
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aman5062/lazyhub/internal/config"
	"github.com/aman5062/lazyhub/internal/github"
)

// defaultClientID is the OAuth App Client ID shipped with lazyhub.
// A device-flow Client ID is NOT a secret (there is no client secret in
// the device flow), so it is safe to embed. Replace with your own
// registered app's ID, or override at runtime with LAZYHUB_CLIENT_ID.
const defaultClientID = "Ov23liOwaFq7EqWmeKBE"

func clientID() string {
	if v := os.Getenv("LAZYHUB_CLIENT_ID"); v != "" {
		return v
	}
	return defaultClientID
}

// LoginWithToken validates a PAT and, if valid, persists it.
func LoginWithToken(ctx context.Context, token string) (*config.Auth, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("empty token")
	}
	c := github.New(token)
	user, scopes, err := c.Me(ctx)
	if err != nil {
		return nil, fmt.Errorf("token rejected by GitHub: %w", err)
	}
	a := &config.Auth{
		Token:     token,
		Method:    "pat",
		Login:     user.Login,
		Scopes:    scopes,
		CreatedAt: time.Now().Unix(),
	}
	if err := config.SaveAuth(a); err != nil {
		return nil, err
	}
	return a, nil
}

// --- Device flow ---

// DeviceCode is what we show the user to authorize the app.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// ErrDeviceFlowUnavailable means no Client ID is configured.
var ErrDeviceFlowUnavailable = errors.New("device flow needs an OAuth App Client ID (set LAZYHUB_CLIENT_ID)")

// RequestDeviceCode starts the device flow and returns the code to display.
func RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	cid := clientID()
	if cid == "" {
		return nil, ErrDeviceFlowUnavailable
	}
	form := url.Values{}
	form.Set("client_id", cid)
	form.Set("scope", "project repo read:org")

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/device/code", strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var dc DeviceCode
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return nil, err
	}
	if dc.DeviceCode == "" {
		return nil, errors.New("github did not return a device code (check Client ID)")
	}
	if dc.Interval == 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

type tokenResp struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

// PollForToken blocks until the user authorizes (or the code expires),
// polling at the interval GitHub asked for. On success it persists the token.
func PollForToken(ctx context.Context, dc *DeviceCode) (*config.Auth, error) {
	cid := clientID()
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	interval := time.Duration(dc.Interval) * time.Second

	for {
		if time.Now().After(deadline) {
			return nil, errors.New("device code expired; run login again")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		form := url.Values{}
		form.Set("client_id", cid)
		form.Set("device_code", dc.DeviceCode)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		var tr tokenResp
		_ = json.NewDecoder(resp.Body).Decode(&tr)
		resp.Body.Close()

		switch tr.Error {
		case "":
			if tr.AccessToken == "" {
				return nil, errors.New("empty access token")
			}
			// Validate + capture login for display.
			c := github.New(tr.AccessToken)
			user, scopes, err := c.Me(ctx)
			if err != nil {
				return nil, err
			}
			if scopes == "" {
				scopes = tr.Scope
			}
			a := &config.Auth{
				Token:     tr.AccessToken,
				Method:    "oauth",
				Login:     user.Login,
				Scopes:    scopes,
				CreatedAt: time.Now().Unix(),
			}
			if err := config.SaveAuth(a); err != nil {
				return nil, err
			}
			return a, nil
		case "authorization_pending":
			// keep polling
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return nil, errors.New("device code expired; run login again")
		case "access_denied":
			return nil, errors.New("authorization denied in browser")
		default:
			return nil, fmt.Errorf("device flow error: %s", tr.Error)
		}
	}
}
