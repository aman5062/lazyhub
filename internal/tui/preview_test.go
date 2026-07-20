package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/aman5062/lazyhub/internal/github"
)

// TestPreview renders the board with fake data so we can eyeball the layout
// without a terminal or a live GitHub token. Run:
//
//	go test ./internal/tui -run Preview -v
func TestPreview(t *testing.T) {
	m := New(github.New("x"), "aman5062")
	m.width, m.height = 120, 30
	m.scr = screenBoard
	m.loading = false
	m.curProject = github.Project{Title: "Ananta HQ Roadmap", Owner: "aman5062"}
	m.statusFld = &github.StatusField{Options: []github.StatusOption{
		{Name: "Todo"}, {Name: "In Progress"}, {Name: "In Review"}, {Name: "Done"},
	}}
	m.rawItems = []github.ProjectItem{
		{Number: 14, Type: "ISSUE", Title: "Wire billing webhooks", Status: "Todo", RepoOwner: "ananta", RepoName: "gateway"},
		{Number: 15, Type: "ISSUE", Title: "Design onboarding flow end to end", Status: "Todo", Assignees: []string{"aman5062"}, RepoOwner: "ananta", RepoName: "home"},
		{Number: 22, Type: "PULL_REQUEST", Title: "Refactor auth middleware", Status: "In Progress", Assignees: []string{"teammate"}, RepoOwner: "ananta", RepoName: "gateway"},
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
