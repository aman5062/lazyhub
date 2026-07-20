// Package config handles where lazyhub stores its state and how it
// persists the GitHub token so the user authenticates only once.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Auth is the persisted credential. We keep it minimal on purpose:
// a token plus how we obtained it (so we know whether we can refresh).
type Auth struct {
	Token     string `json:"token"`
	Method    string `json:"method"`     // "pat" or "oauth"
	Login     string `json:"login"`      // cached username for display
	Scopes    string `json:"scopes"`     // space-separated, best-effort
	CreatedAt int64  `json:"created_at"` // unix seconds
}

// Dir returns ~/.config/lazyhub, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "lazyhub")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func authPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// ErrNoAuth is returned by LoadAuth when the user has never logged in.
var ErrNoAuth = errors.New("no stored credential")

// LoadAuth reads the persisted token. Returns ErrNoAuth if none exists.
func LoadAuth() (*Auth, error) {
	p, err := authPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoAuth
	}
	if err != nil {
		return nil, err
	}
	var a Auth
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	if a.Token == "" {
		return nil, ErrNoAuth
	}
	return &a, nil
}

// SaveAuth writes the credential with 0600 perms (owner read/write only).
// This is the reason you never have to log in twice.
func SaveAuth(a *Auth) error {
	p, err := authPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file then rename for atomicity.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ClearAuth removes the stored credential (logout).
func ClearAuth() error {
	p, err := authPath()
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
