// Package update checks GitHub Releases for a newer lazyhub and can replace
// the running binary in place (`lazyhub upgrade`).
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "aman5062/lazyhub"

// ErrUpToDate means the current build is already the latest release.
var ErrUpToDate = errors.New("already up to date")

// Latest returns the newest release tag (e.g. "v0.1.2").
func Latest(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lazyhub")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("release check: %s", resp.Status)
	}
	var out struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.TagName == "" {
		return "", errors.New("no tag in latest release")
	}
	return out.TagName, nil
}

// IsNewer reports whether latest is a higher semver than current. Dev builds
// (version "dev" or empty) are never considered outdated, to avoid nagging
// during local development.
func IsNewer(current, latest string) bool {
	if current == "" || current == "dev" {
		return false
	}
	return cmpSemver(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(current, "v")) > 0
}

// cmpSemver returns -1/0/1 comparing dotted numeric versions.
func cmpSemver(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var av, bv int
		if i < len(as) {
			av, _ = strconv.Atoi(numPrefix(as[i]))
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(numPrefix(bs[i]))
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

func numPrefix(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}

// assetName is the release archive matching this OS/arch, e.g.
// "lazyhub_linux_amd64.tar.gz" — must match the GoReleaser name template.
func assetName() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("lazyhub_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// SelfUpdate downloads the latest release for this platform and atomically
// replaces the running executable. Returns ErrUpToDate if nothing to do.
func SelfUpdate(ctx context.Context, current string) (string, error) {
	latest, err := Latest(ctx)
	if err != nil {
		return "", err
	}
	if current != "dev" && cmpSemver(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(current, "v")) <= 0 {
		return latest, ErrUpToDate
	}

	exe, err := os.Executable()
	if err != nil {
		return latest, err
	}
	exe, _ = filepath.EvalSymlinks(exe)

	// Fail fast (before a multi-MB download) if we can't write where the
	// binary lives — the usual case for /usr/local/bin owned by root.
	if err := checkWritable(exe); err != nil {
		return latest, err
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, latest, assetName())
	bin, err := downloadBinary(ctx, url)
	if err != nil {
		return latest, err
	}

	// Write next to the target, then rename over it (atomic on the same fs).
	newPath := exe + ".new"
	if err := os.WriteFile(newPath, bin, 0o755); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return latest, permErr(exe)
		}
		return latest, fmt.Errorf("write update to %s: %w", filepath.Dir(exe), err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		os.Remove(newPath)
		if errors.Is(err, os.ErrPermission) {
			return latest, permErr(exe)
		}
		return latest, fmt.Errorf("replace %s: %w", exe, err)
	}
	return latest, nil
}

// checkWritable verifies we can create files in the directory holding exe,
// returning a friendly sudo hint when we can't.
func checkWritable(exe string) error {
	dir := filepath.Dir(exe)
	probe := filepath.Join(dir, ".lazyhub-write-test")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return permErr(exe)
		}
		return fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	f.Close()
	os.Remove(probe)
	return nil
}

// permErr is the actionable message shown when the install location isn't
// writable by the current user.
func permErr(exe string) error {
	return fmt.Errorf("%s is not writable by your user — re-run with elevated permissions:\n    sudo lazyhub update", exe)
}

func downloadBinary(ctx context.Context, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "lazyhub")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(url, ".zip") {
		return extractZip(data)
	}
	return extractTarGz(data)
}

func extractTarGz(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == "lazyhub" {
			return io.ReadAll(tr)
		}
	}
	return nil, errors.New("lazyhub binary not found in archive")
}

func extractZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "lazyhub.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, errors.New("lazyhub.exe not found in archive")
}
