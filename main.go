// lazyhub — a lazygit-style TUI for browsing and operating your GitHub
// repositories. Authenticate once (PAT or OAuth device flow); the token is
// stored locally so you never log in again.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aman5062/lazyhub/internal/config"
	"github.com/aman5062/lazyhub/internal/github"
	"github.com/aman5062/lazyhub/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

// version is overridden at build time via -ldflags "-X main.version=..."
// (GoReleaser sets it to the git tag on release).
var version = "dev"

const usage = `lazyhub — GitHub in your terminal

Usage:
  lazyhub            Launch the TUI (logs you in first if needed)
  lazyhub login      Authenticate with GitHub (PAT or device flow)
  lazyhub logout     Remove the stored credential
  lazyhub whoami     Show the logged-in account
  lazyhub version    Print the version
  lazyhub help       Show this help
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
		fmt.Printf("lazyhub %s\n", version)
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
	m := tui.New(client, a.Login)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fail(err)
	}
}

func whoami() {
	a, err := config.LoadAuth()
	if errors.Is(err, config.ErrNoAuth) {
		fmt.Println("Not logged in. Run: lazyhub login")
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
