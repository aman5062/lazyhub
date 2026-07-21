// Package config handles where lazyhub stores its state and how it
// persists the GitHub token so the user authenticates only once.
//
// Storage strategy: the secret (the token) is kept in the OS keychain when
// one is available — libsecret/GNOME-Keyring on Linux, the Keychain on macOS,
// the Credential Manager on Windows — and the non-secret metadata (login,
// method, scopes) lives in a small JSON file. When no keychain is reachable
// (headless boxes, CI, SSH sessions without a session bus) lazyhub falls back
// to the same 0600 file it always used, so login never fails.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// keychainService / keychainUser identify our secret in the OS keychain.
const (
	keychainService = "lazyhub"
	keychainUser    = "github-token"
)

// Auth is the persisted credential. We keep it minimal on purpose:
// a token plus how we obtained it (so we know whether we can refresh).
type Auth struct {
	Token     string `json:"token,omitempty"` // omitted from disk when in the keychain
	Method    string `json:"method"`          // "pat" or "oauth"
	Login     string `json:"login"`           // cached username for display
	Scopes    string `json:"scopes"`          // space-separated, best-effort
	CreatedAt int64  `json:"created_at"`      // unix seconds
	Keychain  bool   `json:"keychain"`        // true when the token lives in the OS keychain
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

// LoadAuth reads the persisted credential. The metadata comes from the JSON
// file; the token comes from the keychain when it was stored there, else from
// the file (legacy / fallback). Returns ErrNoAuth if none exists.
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
	if a.Keychain {
		tok, kerr := keyring.Get(keychainService, keychainUser)
		if kerr == nil && tok != "" {
			a.Token = tok
		}
		// If the keychain read fails, a.Token stays whatever the file held
		// (normally empty) and we fall through to the empty-token check.
	}
	if a.Token == "" {
		return nil, ErrNoAuth
	}
	return &a, nil
}

// SaveAuth persists the credential. It tries the OS keychain for the token
// first; if that works the file omits the token entirely. If the keychain is
// unavailable it writes the token into the 0600 file as before, so the user
// still only logs in once.
func SaveAuth(a *Auth) error {
	p, err := authPath()
	if err != nil {
		return err
	}

	// Copy so we can blank the token on disk without mutating the caller's value.
	onDisk := *a
	if err := keyring.Set(keychainService, keychainUser, a.Token); err == nil {
		onDisk.Keychain = true
		onDisk.Token = "" // secret lives in the keychain now
	} else {
		onDisk.Keychain = false // fall back to the file
		// Best-effort: don't leave a stale secret behind in the keychain.
		_ = keyring.Delete(keychainService, keychainUser)
	}

	b, err := json.MarshalIndent(&onDisk, "", "  ")
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

// ClearAuth removes the stored credential (logout) from both the keychain
// and the file.
func ClearAuth() error {
	// Remove the keychain secret first; ignore "not found".
	if err := keyring.Delete(keychainService, keychainUser); err != nil &&
		!errors.Is(err, keyring.ErrNotFound) {
		// Non-fatal: still remove the file so the user is logged out.
	}
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
