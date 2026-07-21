package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/aman5062/lazyhub/internal/github"
	"github.com/charmbracelet/bubbles/viewport"
)

// TestPreview renders the board with fake data so we can eyeball the layout
// without a terminal or a live GitHub token. Run:
//
//	go test ./internal/tui -run Preview -v
func TestPreview(t *testing.T) {
	m := New(github.New("x"), "aman5062", "v0.1.2")
	m.width, m.height = 120, 30
	m.scr = screenBoard
	m.loading = false
	m.curProject = github.Project{Title: "Product Roadmap", Owner: "aman5062"}
	m.statusFld = &github.StatusField{Options: []github.StatusOption{
		{Name: "Todo"}, {Name: "In Progress"}, {Name: "In Review"}, {Name: "Done"},
	}}
	m.rawItems = []github.ProjectItem{
		{Number: 14, Type: "ISSUE", Title: "Wire billing webhooks", Status: "Todo", RepoOwner: "acme", RepoName: "gateway"},
		{Number: 15, Type: "ISSUE", Title: "Design onboarding flow end to end", Status: "Todo", Assignees: []string{"aman5062"}, RepoOwner: "acme", RepoName: "home"},
		{Number: 22, Type: "PULL_REQUEST", Title: "Refactor auth middleware", Status: "In Progress", Assignees: []string{"teammate"}, RepoOwner: "acme", RepoName: "gateway"},
		{Number: 31, Type: "ISSUE", Title: "Add rate limiting", Status: "In Progress", Assignees: []string{"aman5062"}},
		{Number: 40, Type: "ISSUE", Title: "Write API docs", Status: "In Review"},
		{Number: 9, Type: "ISSUE", Title: "Set up CI", Status: "Done", Assignees: []string{"aman5062"}},
	}
	m.rebuildColumns()
	m.lastSync = time.Now().Add(-12 * time.Second)

	fmt.Println("\n===== BOARD (120x30) =====")
	fmt.Println(m.View())

	fmt.Println("\n===== HELP OVERLAY =====")
	m.showHelp = true
	fmt.Println(m.View())
}

// TestSplashPreview renders the welcome screen in both its loading and
// ready states so we can eyeball the wordmark and layout.
//
//	go test ./internal/tui -run SplashPreview -v
func TestSplashPreview(t *testing.T) {
	m := New(github.New("x"), "aman5062", "v0.2.0")
	m.width, m.height = 120, 30

	fmt.Println("\n===== SPLASH (loading) =====")
	fmt.Println(m.View())

	m.projectsReady = true
	m.loading = false
	fmt.Println("\n===== SPLASH (ready) =====")
	fmt.Println(m.View())
}

// TestDetailPreview renders the read-only ticket detail view with fake data.
//
//	go test ./internal/tui -run DetailPreview -v
func TestDetailPreview(t *testing.T) {
	m := New(github.New("x"), "aman5062", "v0.1.2")
	m.width, m.height = 120, 34
	m.detailItem = github.ProjectItem{
		Number: 22, Type: "PULL_REQUEST", Title: "Refactor auth middleware",
		State: "OPEN", Status: "In Progress", RepoOwner: "acme", RepoName: "gateway",
		Assignees: []string{"teammate", "aman5062"},
	}
	m.detail = &github.ItemDetail{
		Body: "The current middleware re-parses the token on every request.\n\n" +
			"This PR caches the decoded claims for the request lifetime and adds a\nfast path for anonymous routes. Closes #18.",
		Author:       "teammate",
		CreatedAt:    time.Now().Add(-26 * time.Hour).Format(time.RFC3339),
		Milestone:    "v2.0",
		Labels:       []github.Label{{Name: "enhancement", Color: "a2eeef"}, {Name: "backend", Color: "5319e7"}},
		CommentTotal: 2,
		Comments: []github.Comment{
			{Author: "aman5062", Body: "Nice — can we add a benchmark for the fast path?", CreatedAt: time.Now().Add(-3 * time.Hour).Format(time.RFC3339)},
			{Author: "teammate", Body: "Done, added in the latest commit.", CreatedAt: time.Now().Add(-30 * time.Minute).Format(time.RFC3339)},
		},
	}
	m.detailLoading = false
	m.scr = screenDetail
	w, h := m.detailSize()
	m.detailVP = viewport.New(w, h)
	m.detailVP.SetContent(m.renderDetailBody())

	fmt.Println("\n===== TICKET DETAIL (120x34) =====")
	fmt.Println(m.View())
}
