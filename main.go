// grit — a lazygit-style TUI for browsing and operating your GitHub
// repositories. Authenticate once (PAT or OAuth device flow); the token is
// stored locally so you never log in again.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aman5062/grit/internal/config"
	"github.com/aman5062/grit/internal/github"
	"github.com/aman5062/grit/internal/tui"
	"github.com/aman5062/grit/internal/update"
	tea "github.com/charmbracelet/bubbletea"
)

// version is overridden at build time via -ldflags "-X main.version=..."
// (GoReleaser sets it to the git tag on release).
var version = "dev"

const usage = `grit — GitHub in your terminal

Usage:
  grit            Launch the TUI (logs you in first if needed)
  grit login      Authenticate with GitHub (PAT or device flow)
  grit logout     Remove the stored credential
  grit whoami     Show the logged-in account
  grit update     Update to the latest release (alias: upgrade)
  grit version    Print the version
  grit help       Show this help
`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "", "ui":
		run()
	case "login":
		if _, err := login(context.Background()); err != nil {
			fail(err)
		}
	case "logout":
		if err := config.ClearAuth(); err != nil {
			fail(err)
		}
		fmt.Println("Logged out.")
	case "whoami":
		whoami()
	case "version", "-v", "--version":
		fmt.Printf("grit %s\n", version)
	case "upgrade", "update":
		upgrade()
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// upgrade downloads and installs the latest release over this binary.
func upgrade() {
	fmt.Println("Checking for updates…")
	latest, err := update.SelfUpdate(context.Background(), version)
	if errors.Is(err, update.ErrUpToDate) {
		fmt.Printf("Already on the latest version (%s).\n", latest)
		return
	}
	if err != nil {
		fail(err)
	}
	fmt.Printf("✓ Updated to %s. Restart grit to use it.\n", latest)
}

// run ensures we're authenticated, then launches the TUI.
func run() {
	a, err := config.LoadAuth()
	if errors.Is(err, config.ErrNoAuth) {
		a, err = login(context.Background())
	}
	if err != nil {
		fail(err)
	}
	client := github.New(a.Token)
	m := tui.New(client, a.Login, version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fail(err)
	}
}

func whoami() {
	a, err := config.LoadAuth()
	if errors.Is(err, config.ErrNoAuth) {
		fmt.Println("Not logged in. Run: grit login")
		return
	}
	if err != nil {
		fail(err)
	}
	fmt.Printf("Logged in as @%s (via %s)\n", a.Login, a.Method)
	if a.Scopes != "" {
		fmt.Printf("Scopes: %s\n", a.Scopes)
	}
}

// login shows the arrow-key login screen and returns the credential.
func login(ctx context.Context) (*config.Auth, error) {
	a, err := tui.RunLogin(ctx)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("login cancelled")
	}
	return a, nil
}
