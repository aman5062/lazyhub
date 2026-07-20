package tui

import (
	"hash/fnv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccent = lipgloss.Color("#8B7CF6")
	colorDim    = lipgloss.Color("#6B7280")
	colorFaint  = lipgloss.Color("#4B5563")
	colorGreen  = lipgloss.Color("#22C55E")
	colorYellow = lipgloss.Color("#F59E0B")
	colorRed    = lipgloss.Color("#EF4444")
	colorFg     = lipgloss.Color("#E5E7EB")
	colorInk    = lipgloss.Color("#0B0B10") // dark text for use on bright chips

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Padding(0, 1)

	paneBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFaint)

	paneBorderActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent)

	detailLabel = lipgloss.NewStyle().Foreground(colorDim)
	detailValue = lipgloss.NewStyle().Foreground(colorFg)

	statusBar = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Padding(0, 1)
	helpStyle = lipgloss.NewStyle().Foreground(colorDim)

	privateBadge = lipgloss.NewStyle().Foreground(colorYellow)
	publicBadge  = lipgloss.NewStyle().Foreground(colorGreen)

	cardStyle    = lipgloss.NewStyle().Foreground(colorFg)
	cardSelected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2A2740")).
			Bold(true)

	keycap = lipgloss.NewStyle().
		Foreground(colorInk).
		Background(lipgloss.Color("#9CA3AF")).
		Bold(true).
		Padding(0, 1)
	keyLabel = lipgloss.NewStyle().Foreground(colorDim)

	selectedArrow = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// --- status → color/icon (auto-detected from the column name) ---

// statusPalette is the fallback cycle for columns whose names don't match a
// known keyword, so every board still gets distinct, stable colors.
var statusPalette = []lipgloss.Color{
	"#3B82F6", "#F59E0B", "#A855F7", "#22C55E",
	"#06B6D4", "#EC4899", "#EF4444", "#64748B",
}

// statusColor maps a column name to a color: known workflow keywords get
// meaningful colors; anything else is hashed onto the palette (stable per name).
func statusColor(name string) lipgloss.Color {
	n := strings.ToLower(name)
	switch {
	case containsAny(n, "done", "complete", "closed", "shipped", "merged"):
		return "#22C55E"
	case containsAny(n, "progress", "doing", "active", "wip", "building"):
		return "#F59E0B"
	case containsAny(n, "review", "qa", "testing", "verify"):
		return "#A855F7"
	case containsAny(n, "todo", "to do", "backlog", "new", "open", "ready"):
		return "#3B82F6"
	case containsAny(n, "block", "hold", "waiting", "stuck"):
		return "#EF4444"
	case containsAny(n, "no status", "none"):
		return "#64748B"
	default:
		h := fnv.New32a()
		_, _ = h.Write([]byte(name))
		return statusPalette[int(h.Sum32())%len(statusPalette)]
	}
}

func statusIcon(name string) string {
	n := strings.ToLower(name)
	switch {
	case containsAny(n, "done", "complete", "closed", "shipped", "merged"):
		return "●"
	case containsAny(n, "progress", "doing", "active", "wip", "building"):
		return "◐"
	case containsAny(n, "review", "qa", "testing", "verify"):
		return "◔"
	case containsAny(n, "block", "hold", "waiting", "stuck"):
		return "✕"
	default:
		return "○"
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// --- assignee avatar chips (deterministic color per person) ---

var avatarPalette = []lipgloss.Color{
	"#2563EB", "#7C3AED", "#DB2777", "#DC2626",
	"#B45309", "#15803D", "#0E7490", "#4F46E5",
}

func userColor(login string) lipgloss.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(login))
	return avatarPalette[int(h.Sum32())%len(avatarPalette)]
}

// initials returns up to two uppercase letters for an avatar chip.
func initials(login string) string {
	login = strings.TrimPrefix(login, "@")
	parts := strings.FieldsFunc(login, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	switch {
	case len(parts) == 0:
		return "?"
	case len(parts) == 1:
		s := parts[0]
		if len(s) >= 2 {
			return strings.ToUpper(s[:2])
		}
		return strings.ToUpper(s)
	default:
		return strings.ToUpper(string(parts[0][0]) + string(parts[1][0]))
	}
}

// avatarChip renders a small colored badge like " AK " for a user.
func avatarChip(login string) string {
	return lipgloss.NewStyle().
		Background(userColor(login)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1).
		Render(initials(login))
}

// columnHeaderPill renders a filled, colored header for a status column.
func columnHeaderPill(name string, count, width int, active bool) string {
	c := statusColor(name)
	label := statusIcon(name) + " " + name
	countStr := lipgloss.NewStyle().Faint(true).Render(" " + itoa(count))
	st := lipgloss.NewStyle().
		Background(c).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Width(width).
		Padding(0, 1)
	if !active {
		st = st.Faint(true)
	}
	return st.Render(label + countStr)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
