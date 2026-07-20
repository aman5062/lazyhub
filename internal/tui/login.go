package tui

import (
	"context"

	"github.com/aman5062/lazyhub/internal/auth"
	"github.com/aman5062/lazyhub/internal/config"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type loginState int

const (
	stChoose loginState = iota
	stToken
	stDevice
	stWorking
	stDone
)

type loginMethod struct {
	title string
	desc  string
}

var loginMethods = []loginMethod{
	{"Personal Access Token", "Paste a token · set \"No expiration\" for a permanent login"},
	{"Browser device flow", "Approve in the browser — no token to copy-paste"},
}

// LoginModel is a small arrow-key login screen shown before the main TUI.
type LoginModel struct {
	ctx    context.Context
	state  loginState
	cursor int
	input  textinput.Model
	spin   spinner.Model
	device *auth.DeviceCode
	auth   *config.Auth
	err    error
	note   string
	width  int
}

func newLoginModel(ctx context.Context) LoginModel {
	ti := textinput.New()
	ti.Placeholder = "ghp_… paste your token"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 255
	ti.Width = 44

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return LoginModel{ctx: ctx, state: stChoose, input: ti, spin: sp}
}

// RunLogin shows the interactive login screen and returns the credential.
func RunLogin(ctx context.Context) (*config.Auth, error) {
	res, err := tea.NewProgram(newLoginModel(ctx), tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	m := res.(LoginModel)
	return m.auth, m.err
}

// --- messages ---

type loginDoneMsg struct {
	auth *config.Auth
	err  error
}
type deviceCodeMsg struct {
	dc  *auth.DeviceCode
	err error
}

func (m LoginModel) Init() tea.Cmd { return textinput.Blink }

func (m LoginModel) tokenLoginCmd(token string) tea.Cmd {
	return func() tea.Msg {
		a, err := auth.LoginWithToken(m.ctx, token)
		return loginDoneMsg{auth: a, err: err}
	}
}

func (m LoginModel) requestDeviceCmd() tea.Cmd {
	return func() tea.Msg {
		dc, err := auth.RequestDeviceCode(m.ctx)
		return deviceCodeMsg{dc: dc, err: err}
	}
}

func (m LoginModel) pollDeviceCmd(dc *auth.DeviceCode) tea.Cmd {
	return func() tea.Msg {
		a, err := auth.PollForToken(m.ctx, dc)
		return loginDoneMsg{auth: a, err: err}
	}
}

func (m LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case deviceCodeMsg:
		if msg.err != nil {
			m.state = stChoose
			m.note = msg.err.Error()
			return m, nil
		}
		m.device = msg.dc
		m.state = stDevice
		return m, tea.Batch(m.spin.Tick, m.pollDeviceCmd(msg.dc))

	case loginDoneMsg:
		if msg.err != nil {
			// Return to the relevant entry screen with the error shown.
			m.note = msg.err.Error()
			if m.device != nil {
				m.state = stChoose
			} else {
				m.state = stToken
				m.input.Focus()
			}
			return m, nil
		}
		m.auth = msg.auth
		m.state = stDone
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.state == stToken {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m LoginModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	switch m.state {
	case stChoose:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(loginMethods)-1 {
				m.cursor++
			}
		case "enter":
			m.note = ""
			if m.cursor == 0 {
				m.state = stToken
				m.input.Focus()
				return m, textinput.Blink
			}
			m.state = stWorking
			return m, tea.Batch(m.spin.Tick, m.requestDeviceCmd())
		}
	case stToken:
		switch msg.String() {
		case "esc":
			m.state = stChoose
			m.input.Blur()
			return m, nil
		case "enter":
			tok := m.input.Value()
			if tok == "" {
				m.note = "paste a token first"
				return m, nil
			}
			m.state = stWorking
			m.note = ""
			return m, tea.Batch(m.spin.Tick, m.tokenLoginCmd(tok))
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case stDevice:
		if msg.String() == "esc" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m LoginModel) View() string {
	logo := titleStyle.Render(" lazyhub ") + lipgloss.NewStyle().
		Foreground(colorDim).Render("  GitHub Projects in your terminal")

	var body string
	switch m.state {
	case stChoose:
		body = m.chooseView()
	case stToken:
		body = m.tokenView()
	case stWorking:
		body = "\n  " + m.spin.View() + " Talking to GitHub…"
	case stDevice:
		body = m.deviceView()
	case stDone:
		body = "\n  " + publicBadge.Render("✓ Logged in")
	}

	note := ""
	if m.note != "" {
		note = "\n\n  " + lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+m.note)
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(logo + "\n\n" + body + note)
}

func (m LoginModel) chooseView() string {
	var b []string
	b = append(b, lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render("How do you want to log in?"), "")
	for i, opt := range loginMethods {
		cursor := "  "
		titleSt := lipgloss.NewStyle().Foreground(colorFg)
		descSt := lipgloss.NewStyle().Foreground(colorDim)
		if i == m.cursor {
			cursor = selectedArrow.Render("❯ ")
			titleSt = titleSt.Foreground(colorAccent).Bold(true)
		}
		b = append(b, cursor+titleSt.Render(opt.title))
		b = append(b, "    "+descSt.Render(opt.desc), "")
	}
	b = append(b, helpStyle.Render("↑/↓ choose · enter select · ctrl+c quit"))
	b = append(b, "", detailLabel.Render("Note: GitHub removed username/password API auth in 2020 — a token is required."))
	return lipgloss.JoinVertical(lipgloss.Left, b...)
}

func (m LoginModel) tokenView() string {
	lines := []string{
		lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render("Paste a Personal Access Token"),
		"",
		detailLabel.Render("Create at ") + detailValue.Render("https://github.com/settings/tokens/new"),
		detailLabel.Render("Scopes: ") + detailValue.Render("project, repo, read:org"),
		detailLabel.Render("Expiration: ") + detailValue.Render("\"No expiration\" for a permanent login"),
		"",
		m.input.View(),
		"",
		helpStyle.Render("enter submit · esc back · ctrl+c quit"),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m LoginModel) deviceView() string {
	lines := []string{
		lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render("Authorize in your browser"),
		"",
		detailLabel.Render("1. Open  ") + detailValue.Render(m.device.VerificationURI),
		detailLabel.Render("2. Enter ") + lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(m.device.UserCode),
		"",
		"  " + m.spin.View() + " Waiting for authorization…",
		"",
		helpStyle.Render("esc cancel"),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
