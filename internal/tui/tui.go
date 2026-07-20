// Package tui is the lazyhub interface: a horizontal kanban board over your
// GitHub Projects. Columns are your real Status options (synced from GitHub),
// laid out side by side. Move between columns and cards with the arrow keys;
// assign people, move tickets between columns, and filter to your own work.
//
// Screens:
//   - Projects:  list of your boards (personal + orgs). Enter opens a board.
//   - Board:     kanban columns. Act on the selected card.
//   - Assignee picker (a): toggle who's assigned.
//   - Status picker   (s): move the card to another column.
//   - Help overlay    (?): all keybindings.
package tui

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/aman5062/lazyhub/internal/config"
	"github.com/aman5062/lazyhub/internal/github"
	"github.com/aman5062/lazyhub/internal/update"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenProjects screen = iota
	screenBoard
	screenAssignee
	screenStatus    // pick an option for the current field (Status via `s`, or any field via `p`)
	screenFieldPick // pick which single-select field to edit (via `p`)
	screenInput     // free text: create a draft ticket, or add a comment
)

type inputPurpose int

const (
	inputCreate inputPurpose = iota
	inputComment
)

const (
	minColWidth = 24

	// autoSyncInterval is how often the board silently re-fetches so tickets
	// added by teammates appear without a manual refresh. A local CLI can't
	// receive GitHub webhooks (no public endpoint), so polling is the
	// pragmatic "live" mechanism — same approach as k9s / lazygit.
	autoSyncInterval = 30 * time.Second
)

// --- list item adapters ---

type projItem struct{ p github.Project }

func (i projItem) Title() string { return i.p.Title }
func (i projItem) Description() string {
	d := i.p.Description
	if d == "" {
		d = "—"
	}
	return fmt.Sprintf("@%s · #%d · %s", i.p.Owner, i.p.Number, d)
}
func (i projItem) FilterValue() string { return i.p.Title + " " + i.p.Owner }

type pickItem struct {
	label    string
	sub      string
	selected bool
	id       string
}

func (i pickItem) Title() string {
	mark := "  "
	if i.selected {
		mark = publicBadge.Render("✓ ")
	}
	return mark + i.label
}
func (i pickItem) Description() string { return i.sub }
func (i pickItem) FilterValue() string { return i.label }

// fieldItem adapts a single-select field for the "which field?" picker.
type fieldItem struct{ f github.SingleSelectField }

func (i fieldItem) Title() string { return i.f.Name }
func (i fieldItem) Description() string {
	names := make([]string, 0, len(i.f.Options))
	for _, o := range i.f.Options {
		names = append(names, o.Name)
	}
	return strings.Join(names, " · ")
}
func (i fieldItem) FilterValue() string { return i.f.Name }

// boardColumn is one status column with its cards.
type boardColumn struct {
	name  string
	items []github.ProjectItem
}

// --- messages ---

type projectsLoadedMsg struct {
	projects []github.Project
	err      error
}
type boardLoadedMsg struct {
	items  []github.ProjectItem
	status *github.StatusField
	err    error
	silent bool // background auto-sync: don't blank the board or reset cursor
}

type updateAvailMsg struct{ version string }

type autoSyncTickMsg struct{}

func scheduleSync() tea.Cmd {
	return tea.Tick(autoSyncInterval, func(time.Time) tea.Msg { return autoSyncTickMsg{} })
}
type assigneesLoadedMsg struct {
	logins []string
	err    error
}
type fieldsLoadedMsg struct {
	fields []github.SingleSelectField
	err    error
}
type actionMsg struct {
	text   string
	kind   string // ok | err | info
	reload bool
}

func clearStatusMsg() tea.Msg { return actionMsg{} }

// --- model ---

type Model struct {
	client      *github.Client
	login       string
	version     string
	updateAvail string // set to the newer version tag if one is available
	width       int
	height      int

	scr     screen
	loading bool
	err     error
	status   string
	statKnd  string
	showHelp bool
	spin     spinner.Model
	lastSync      time.Time
	syncing       bool
	confirmLogout bool

	projects list.Model

	// board state
	curProject github.Project
	rawItems   []github.ProjectItem
	columns    []boardColumn
	statusFld  *github.StatusField
	colCursor  int
	cardCursor int
	filterMine bool

	picker       list.Model
	pendingField *github.SingleSelectField // field whose option is being picked
	input        textinput.Model
	inputPurpose inputPurpose
}

func New(client *github.Client, login, version string) Model {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(colorFg).BorderForeground(colorAccent)

	l := list.New(nil, d, 0, 0)
	l.Title = "Project boards"
	l.Styles.Title = titleStyle
	l.SetShowHelp(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return Model{
		client:   client,
		login:    login,
		version:  version,
		scr:      screenProjects,
		loading:  true,
		projects: l,
		spin:     sp,
	}
}

func (m Model) Init() tea.Cmd {
	// One long-lived sync ticker for the app's lifetime; it only does work
	// while a board is open and idle (see the autoSyncTickMsg handler).
	return tea.Batch(m.loadProjects(), m.spin.Tick, scheduleSync(), m.checkUpdate())
}

// checkUpdate asks GitHub (once, in the background) whether a newer release
// exists, so we can show an unobtrusive notice.
func (m Model) checkUpdate() tea.Cmd {
	ver := m.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		latest, err := update.Latest(ctx)
		if err != nil || !update.IsNewer(ver, latest) {
			return updateAvailMsg{}
		}
		return updateAvailMsg{version: latest}
	}
}

// --- commands ---

func (m Model) loadProjects() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		p, err := m.client.ListProjects(ctx)
		return projectsLoadedMsg{projects: p, err: err}
	}
}

func (m Model) loadBoard(p github.Project, silent bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		items, err := m.client.ListProjectItems(ctx, p.ID)
		if err != nil {
			return boardLoadedMsg{err: err, silent: silent}
		}
		sf, _ := m.client.GetStatusField(ctx, p.ID)
		return boardLoadedMsg{items: items, status: sf, silent: silent}
	}
}

func (m Model) loadAssignees(it github.ProjectItem) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		logins, err := m.client.ListAssignableUsers(ctx, it.RepoOwner, it.RepoName)
		return assigneesLoadedMsg{logins: logins, err: err}
	}
}

func (m Model) loadFields() tea.Cmd {
	proj := m.curProject
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		fields, err := m.client.ListSingleSelectFields(ctx, proj.ID)
		return fieldsLoadedMsg{fields: fields, err: err}
	}
}

// newTextInput builds the input for creating a ticket / adding a comment.
func (m *Model) openInput(purpose inputPurpose, placeholder string) {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 500
	ti.Width = 50
	ti.Focus()
	m.input = ti
	m.inputPurpose = purpose
	m.scr = screenInput
}

// --- board helpers ---

// rebuildColumns applies the current filter and lays items into columns whose
// order matches the project's real Status options (synced from GitHub), plus
// any leftover statuses (e.g. "No Status") appended at the end.
func (m *Model) rebuildColumns() {
	items := m.rawItems
	if m.filterMine {
		f := make([]github.ProjectItem, 0, len(items))
		for _, it := range items {
			for _, a := range it.Assignees {
				if a == m.login {
					f = append(f, it)
					break
				}
			}
		}
		items = f
	}

	byStatus := map[string][]github.ProjectItem{}
	for _, it := range items {
		byStatus[it.Status] = append(byStatus[it.Status], it)
	}

	var order []string
	seen := map[string]bool{}
	if m.statusFld != nil {
		for _, o := range m.statusFld.Options {
			order = append(order, o.Name)
			seen[o.Name] = true
		}
	}
	// preserve appearance order for any statuses not in the field (e.g. No Status)
	for _, it := range items {
		if !seen[it.Status] {
			seen[it.Status] = true
			order = append(order, it.Status)
		}
	}

	cols := make([]boardColumn, 0, len(order))
	for _, name := range order {
		cols = append(cols, boardColumn{name: name, items: byStatus[name]})
	}
	m.columns = cols
	m.clampBoard()
}

func (m *Model) clampBoard() {
	if len(m.columns) == 0 {
		m.colCursor, m.cardCursor = 0, 0
		return
	}
	if m.colCursor >= len(m.columns) {
		m.colCursor = len(m.columns) - 1
	}
	if m.colCursor < 0 {
		m.colCursor = 0
	}
	n := len(m.columns[m.colCursor].items)
	if m.cardCursor >= n {
		m.cardCursor = n - 1
	}
	if m.cardCursor < 0 {
		m.cardCursor = 0
	}
}

// restoreSelection moves the cursor back onto the card with itemID after a
// refresh, so auto-sync never makes the selection jump.
func (m *Model) restoreSelection(itemID string) {
	for ci, col := range m.columns {
		for ii, it := range col.items {
			if it.ItemID == itemID {
				m.colCursor, m.cardCursor = ci, ii
				return
			}
		}
	}
	m.clampBoard()
}

// currentFieldValue returns the item's current value for a field, when known.
// We only track the Status value per item, so other fields start unmarked.
func currentFieldValue(it github.ProjectItem, f github.SingleSelectField) string {
	if f.Name == "Status" {
		return it.Status
	}
	return ""
}

func (m *Model) selectedItem() (github.ProjectItem, bool) {
	if m.colCursor < 0 || m.colCursor >= len(m.columns) {
		return github.ProjectItem{}, false
	}
	col := m.columns[m.colCursor]
	if m.cardCursor < 0 || m.cardCursor >= len(col.items) {
		return github.ProjectItem{}, false
	}
	return col.items[m.cardCursor], true
}

// --- update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case updateAvailMsg:
		m.updateAvail = msg.version
		return m, nil

	case spinner.TickMsg:
		// Only animate while something is actually loading, to stay idle-quiet.
		if !m.loading && !m.syncing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case autoSyncTickMsg:
		cmds := []tea.Cmd{scheduleSync()} // keep the ticker alive
		// Only sync when viewing a board and not busy / not in a modal.
		if m.scr == screenBoard && !m.loading && !m.syncing {
			m.syncing = true
			cmds = append(cmds, m.loadBoard(m.curProject, true), m.spin.Tick)
		}
		return m, tea.Batch(cmds...)

	case projectsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.projects))
		for _, p := range msg.projects {
			items = append(items, projItem{p})
		}
		return m, m.projects.SetItems(items)

	case boardLoadedMsg:
		m.loading = false
		m.syncing = false
		if msg.err != nil {
			if msg.silent {
				// Don't tear down the visible board on a background hiccup.
				return m, m.tempStatus("sync failed: "+msg.err.Error(), "err")
			}
			m.err = msg.err
			return m, nil
		}
		// Preserve the user's selected card across refresh (match by ID).
		selID := ""
		if it, ok := m.selectedItem(); ok {
			selID = it.ItemID
		}
		oldIDs := map[string]bool{}
		for _, it := range m.rawItems {
			oldIDs[it.ItemID] = true
		}
		m.rawItems = msg.items
		m.statusFld = msg.status
		m.rebuildColumns()
		if selID != "" {
			m.restoreSelection(selID)
		}
		m.lastSync = time.Now()
		if msg.silent {
			newCount := 0
			for _, it := range msg.items {
				if !oldIDs[it.ItemID] {
					newCount++
				}
			}
			if newCount > 0 {
				return m, m.tempStatus(fmt.Sprintf("⟳ %d new ticket(s) synced", newCount), "ok")
			}
			return m, nil
		}
		return m, m.tempStatus(fmt.Sprintf("Loaded %d tickets", len(msg.items)), "ok")

	case assigneesLoadedMsg:
		if msg.err != nil {
			return m, m.tempStatus("assignees: "+msg.err.Error(), "err")
		}
		it, _ := m.selectedItem()
		assigned := map[string]bool{}
		for _, a := range it.Assignees {
			assigned[a] = true
		}
		var li []list.Item
		for _, login := range msg.logins {
			li = append(li, pickItem{label: login, selected: assigned[login]})
		}
		m.picker = m.newPicker("Assign to (enter toggles)", li)
		m.scr = screenAssignee
		m.resize()
		return m, nil

	case fieldsLoadedMsg:
		if msg.err != nil {
			return m, m.tempStatus("fields: "+msg.err.Error(), "err")
		}
		var li []list.Item
		for _, f := range msg.fields {
			if len(f.Options) == 0 {
				continue
			}
			li = append(li, fieldItem{f})
		}
		if len(li) == 0 {
			return m, m.tempStatus("no editable single-select fields on this board", "err")
		}
		m.picker = m.newPicker("Which field?", li)
		m.scr = screenFieldPick
		m.resize()
		return m, nil

	case actionMsg:
		m.status, m.statKnd = msg.text, msg.kind
		var cmds []tea.Cmd
		if msg.text != "" {
			cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearStatusMsg() }))
		}
		if msg.reload {
			// Refresh in the background so the board doesn't blank after an action.
			m.syncing = true
			cmds = append(cmds, m.loadBoard(m.curProject, true), m.spin.Tick)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.scr {
	case screenProjects:
		m.projects, cmd = m.projects.Update(msg)
	case screenAssignee, screenStatus, screenFieldPick:
		m.picker, cmd = m.picker.Update(msg)
	case screenInput:
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Help overlay swallows keys.
	if m.showHelp {
		if key == "?" || key == "esc" || key == "q" {
			m.showHelp = false
		}
		return m, nil
	}
	if key == "?" {
		m.showHelp = true
		return m, nil
	}

	filtering := (m.scr == screenProjects && m.projects.FilterState() == list.Filtering) ||
		((m.scr == screenAssignee || m.scr == screenStatus || m.scr == screenFieldPick) && m.picker.FilterState() == list.Filtering)

	if !filtering && (key == "ctrl+c") {
		return m, tea.Quit
	}

	// Logout confirmation is a small two-step guard, available anywhere.
	if m.confirmLogout {
		if key == "X" {
			return m, tea.Sequence(
				func() tea.Msg { _ = config.ClearAuth(); return nil },
				tea.Quit,
			)
		}
		m.confirmLogout = false
		if key == "esc" {
			return m, m.tempStatus("Logout cancelled", "info")
		}
	} else if key == "X" {
		m.confirmLogout = true
		return m, m.tempStatus("⚠ Press X again to log out · esc cancels", "err")
	}

	switch m.scr {
	case screenProjects:
		if !filtering && key == "q" {
			return m, tea.Quit
		}
		if !filtering && key == "o" {
			if it, ok := m.projects.SelectedItem().(projItem); ok && it.p.URL != "" {
				return m, openBrowser(it.p.URL)
			}
		}
		if !filtering && key == "enter" {
			if it, ok := m.projects.SelectedItem().(projItem); ok {
				m.curProject = it.p
				m.scr = screenBoard
				m.loading = true
				m.colCursor, m.cardCursor = 0, 0
				return m, tea.Batch(m.loadBoard(it.p, false), m.spin.Tick)
			}
		}
		var cmd tea.Cmd
		m.projects, cmd = m.projects.Update(msg)
		return m, cmd

	case screenBoard:
		switch key {
		case "q", "esc":
			m.scr = screenProjects
		case "left", "h":
			if m.colCursor > 0 {
				m.colCursor--
				m.cardCursor = 0
			}
		case "right", "l":
			if m.colCursor < len(m.columns)-1 {
				m.colCursor++
				m.cardCursor = 0
			}
		case "up", "k":
			if m.cardCursor > 0 {
				m.cardCursor--
			}
		case "down", "j":
			if col := m.curColumn(); col != nil && m.cardCursor < len(col.items)-1 {
				m.cardCursor++
			}
		case "r":
			// Manual refresh: background sync, keep the board visible.
			m.syncing = true
			return m, tea.Batch(m.loadBoard(m.curProject, true), m.spin.Tick)
		case "m":
			m.filterMine = !m.filterMine
			m.rebuildColumns()
			label := "Showing all tickets"
			if m.filterMine {
				label = "Showing only my tickets"
			}
			return m, m.tempStatus(label, "info")
		case "o":
			if it, ok := m.selectedItem(); ok && it.URL != "" {
				return m, openBrowser(it.URL)
			}
		case "a":
			if it, ok := m.selectedItem(); ok {
				if it.Number == 0 {
					return m, m.tempStatus("draft items can't be assigned", "err")
				}
				return m, m.loadAssignees(it)
			}
		case "s":
			if _, ok := m.selectedItem(); ok {
				return m.openStatusPicker()
			}
		case "p":
			if _, ok := m.selectedItem(); ok {
				return m, m.loadFields()
			}
		case "n":
			m.openInput(inputCreate, "New ticket title…")
			return m, textinput.Blink
		case "c":
			if it, ok := m.selectedItem(); ok {
				if it.Number == 0 {
					return m, m.tempStatus("draft items have no comments", "err")
				}
				m.openInput(inputComment, "Write a comment…")
				return m, textinput.Blink
			}
		}
		return m, nil

	case screenFieldPick:
		switch key {
		case "esc":
			m.scr = screenBoard
			return m, nil
		case "enter":
			if !filtering {
				if fi, ok := m.picker.SelectedItem().(fieldItem); ok {
					it, _ := m.selectedItem()
					f := fi.f
					m.pendingField = &f
					m.openOptionPicker(currentFieldValue(it, f))
					return m, nil
				}
			}
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd

	case screenInput:
		switch key {
		case "esc":
			m.scr = screenBoard
			m.input.Blur()
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.input.Value())
			if val == "" {
				return m, m.tempStatus("type something first", "err")
			}
			m.scr = screenBoard
			if m.inputPurpose == inputCreate {
				return m, m.createTicket(val)
			}
			if it, ok := m.selectedItem(); ok {
				return m, m.postComment(it, val)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case screenAssignee:
		switch key {
		case "esc":
			m.scr = screenBoard
			return m, nil
		case "enter":
			if !filtering {
				return m.toggleAssignee()
			}
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd

	case screenStatus:
		switch key {
		case "esc":
			m.scr = screenBoard
			return m, nil
		case "enter":
			if !filtering {
				return m.applyStatus()
			}
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}
	return m, nil
}

// syncIndicator renders the auto-sync state shown next to the board title.
func (m Model) syncIndicator() string {
	if m.syncing {
		return m.spin.View() + " syncing…"
	}
	if m.lastSync.IsZero() {
		return ""
	}
	d := time.Since(m.lastSync)
	if d < time.Minute {
		return fmt.Sprintf("✓ synced %ds ago · auto every %ds", int(d.Seconds()), int(autoSyncInterval.Seconds()))
	}
	return fmt.Sprintf("✓ synced %dm ago", int(d.Minutes()))
}

func (m *Model) curColumn() *boardColumn {
	if m.colCursor >= 0 && m.colCursor < len(m.columns) {
		return &m.columns[m.colCursor]
	}
	return nil
}

func (m Model) openStatusPicker() (tea.Model, tea.Cmd) {
	if m.statusFld == nil || len(m.statusFld.Options) == 0 {
		return m, m.tempStatus("this board has no Status columns", "err")
	}
	it, _ := m.selectedItem()
	m.pendingField = &github.SingleSelectField{
		ID: m.statusFld.FieldID, Name: "Status", Options: m.statusFld.Options,
	}
	m.openOptionPicker(it.Status)
	return m, nil
}

// openOptionPicker shows the options of m.pendingField, marking the current one.
func (m *Model) openOptionPicker(current string) {
	var li []list.Item
	for _, o := range m.pendingField.Options {
		li = append(li, pickItem{label: o.Name, id: o.ID, selected: o.Name == current})
	}
	m.picker = m.newPicker("Set "+m.pendingField.Name, li)
	m.scr = screenStatus
	m.resize()
}

func (m Model) toggleAssignee() (tea.Model, tea.Cmd) {
	pi, ok := m.picker.SelectedItem().(pickItem)
	if !ok {
		return m, nil
	}
	it, _ := m.selectedItem()
	login := pi.label
	wasAssigned := pi.selected
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var err error
		if wasAssigned {
			err = m.client.RemoveAssignees(ctx, it.RepoOwner, it.RepoName, it.Number, []string{login})
		} else {
			err = m.client.AddAssignees(ctx, it.RepoOwner, it.RepoName, it.Number, []string{login})
		}
		if err != nil {
			return actionMsg{text: "assign failed: " + err.Error(), kind: "err"}
		}
		verb := "Assigned"
		if wasAssigned {
			verb = "Unassigned"
		}
		return actionMsg{text: fmt.Sprintf("%s @%s", verb, login), kind: "ok", reload: true}
	}
}

func (m Model) applyStatus() (tea.Model, tea.Cmd) {
	pi, ok := m.picker.SelectedItem().(pickItem)
	if !ok || m.pendingField == nil {
		return m, nil
	}
	it, _ := m.selectedItem()
	proj := m.curProject
	fieldID := m.pendingField.ID
	fieldName := m.pendingField.Name
	m.scr = screenBoard
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := m.client.SetItemStatus(ctx, proj.ID, it.ItemID, fieldID, pi.id); err != nil {
			return actionMsg{text: "update failed: " + err.Error(), kind: "err"}
		}
		return actionMsg{text: fmt.Sprintf("%s → %s", fieldName, pi.label), kind: "ok", reload: true}
	}
}

// createTicket adds a draft ticket to the board from the input text.
func (m Model) createTicket(title string) tea.Cmd {
	proj := m.curProject
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := m.client.AddDraftIssue(ctx, proj.ID, title, ""); err != nil {
			return actionMsg{text: "create failed: " + err.Error(), kind: "err"}
		}
		return actionMsg{text: "Created draft ticket", kind: "ok", reload: true}
	}
}

// postComment adds a comment to the selected issue/PR.
func (m Model) postComment(it github.ProjectItem, body string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := m.client.AddComment(ctx, it.RepoOwner, it.RepoName, it.Number, body); err != nil {
			return actionMsg{text: "comment failed: " + err.Error(), kind: "err"}
		}
		return actionMsg{text: "Comment posted", kind: "ok"}
	}
}

func (m Model) tempStatus(text, kind string) tea.Cmd {
	return func() tea.Msg { return actionMsg{text: text, kind: kind} }
}

// pickerDelegate renders picker rows with a colored dot, a ❯ cursor, and a
// "✓ current" marker — much cleaner than the default list rows.
type pickerDelegate struct{}

func (pickerDelegate) Height() int                             { return 1 }
func (pickerDelegate) Spacing() int                            { return 0 }
func (pickerDelegate) Update(tea.Msg, *list.Model) tea.Cmd     { return nil }
func (d pickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	cur := index == m.Index()
	cursor := "  "
	if cur {
		cursor = selectedArrow.Render("❯ ")
	}
	nameSt := lipgloss.NewStyle().Foreground(colorFg)
	if cur {
		nameSt = nameSt.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	}
	switch it := item.(type) {
	case pickItem:
		dot := lipgloss.NewStyle().Foreground(statusColor(it.label)).Render("●")
		row := cursor + dot + " " + nameSt.Render(it.label)
		if it.selected {
			row += "   " + publicBadge.Render("✓ current")
		}
		fmt.Fprint(w, row)
	case fieldItem:
		row := cursor + lipgloss.NewStyle().Foreground(colorAccent).Render("◈ ") +
			nameSt.Render(it.f.Name) + "  " + detailLabel.Render(truncate(it.Description(), 34))
		fmt.Fprint(w, row)
	default:
		fmt.Fprint(w, cursor+nameSt.Render(item.FilterValue()))
	}
}

func (m Model) newPicker(title string, items []list.Item) list.Model {
	l := list.New(items, pickerDelegate{}, 0, 0)
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	w, h := m.pickerSize()
	l.SetSize(w, h)
	return l
}

// --- layout ---

func (m *Model) resize() {
	bodyH := m.height - 4
	if bodyH < 3 {
		bodyH = 3
	}
	m.projects.SetSize(m.width-2, bodyH)
	if m.scr == screenAssignee || m.scr == screenStatus || m.scr == screenFieldPick {
		w, h := m.pickerSize()
		m.picker.SetSize(w, h)
	}
}

func (m Model) pickerSize() (int, int) {
	w := m.width * 60 / 100
	if w < 30 {
		w = 30
	}
	h := m.height - 8
	if h < 4 {
		h = 4
	}
	return w, h
}

// --- view ---

func (m Model) View() string {
	if m.width == 0 {
		return "starting lazyhub…"
	}
	if m.showHelp {
		return m.helpOverlay()
	}
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(colorRed).Padding(1, 2).Render(
			"Error:\n\n" + m.err.Error() +
				"\n\nTip: Projects need the `project` scope on your token.\nPress q to quit.")
	}

	scope := "Projects"
	if m.filterMine {
		scope = "Projects · mine"
	}
	header := titleStyle.Render("lazyhub") + statusBar.Render(" @"+m.login+"  ·  "+scope)
	if m.updateAvail != "" {
		header += lipgloss.NewStyle().Foreground(colorYellow).Bold(true).
			Render("  ⬆ " + m.updateAvail + " available — run: lazyhub upgrade")
	}

	var body string
	switch m.scr {
	case screenProjects:
		if m.loading {
			body = "\n  " + m.spin.View() + " Loading your project boards…"
		} else {
			body = m.projects.View()
		}
	case screenBoard:
		body = m.boardView()
	case screenAssignee, screenStatus, screenFieldPick:
		body = paneBorderActive.Padding(0, 1).Render(m.picker.View())
	case screenInput:
		body = m.inputView()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, m.statusView(), m.helpBar())
}

func (m Model) inputView() string {
	heading := "Create a draft ticket"
	hint := "Adds a draft card to this board."
	if m.inputPurpose == inputComment {
		heading = "Add a comment"
		if it, ok := m.selectedItem(); ok {
			hint = fmt.Sprintf("On %s/%s #%d", it.RepoOwner, it.RepoName, it.Number)
		}
	}
	box := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(heading),
		detailLabel.Render(hint),
		"",
		m.input.View(),
		"",
		helpStyle.Render("enter submit · esc cancel"),
	)
	return "\n" + paneBorderActive.Padding(1, 2).Render(box)
}

func (m Model) boardView() string {
	if m.loading {
		return "\n  " + m.spin.View() + " Loading tickets…"
	}
	total := 0
	for _, c := range m.columns {
		total += len(c.items)
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).
		Render(fmt.Sprintf("  %s  (@%s) · %d tickets", m.curProject.Title, m.curProject.Owner, total)) +
		detailLabel.Render("   "+m.syncIndicator())
	if len(m.columns) == 0 {
		hint := "  No tickets match."
		if !m.filterMine {
			hint = "  This board has no tickets yet."
		}
		return title + "\n\n" + hint
	}

	bodyH := m.height - 5
	if bodyH < 6 {
		bodyH = 6
	}

	// How many columns fit; window so the active column is visible.
	fit := m.width / (minColWidth + 1)
	if fit < 1 {
		fit = 1
	}
	visible := len(m.columns)
	if visible > fit {
		visible = fit
	}
	colW := m.width/visible - 1
	if colW > 44 {
		colW = 44
	}
	if colW < minColWidth {
		colW = minColWidth
	}

	scroll := 0
	if m.colCursor >= visible {
		scroll = m.colCursor - visible + 1
	}
	end := scroll + visible
	if end > len(m.columns) {
		end = len(m.columns)
	}

	rendered := make([]string, 0, visible)
	for i := scroll; i < end; i++ {
		rendered = append(rendered, m.renderColumn(i, colW, bodyH))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	nav := ""
	if len(m.columns) > visible {
		nav = detailLabel.Render(fmt.Sprintf("   columns %d–%d of %d  (← →)", scroll+1, end, len(m.columns)))
	}
	return title + nav + "\n" + m.distributionBar(total) + "\n" + board
}

// distributionBar renders a full-width stacked bar coloured by column, with a
// "% done" readout — a quick visual pulse of where the work sits.
func (m Model) distributionBar(total int) string {
	barW := m.width - 4
	if barW < 10 {
		barW = 10
	}
	if total == 0 {
		return "  " + lipgloss.NewStyle().Foreground(colorFaint).Render(strings.Repeat("░", barW))
	}

	var b strings.Builder
	b.WriteString("  ")
	used, doneCount := 0, 0
	for i, col := range m.columns {
		seg := len(col.items) * barW / total
		if i == len(m.columns)-1 {
			seg = barW - used // give the remainder to the last column
		}
		if seg < 0 {
			seg = 0
		}
		used += seg
		b.WriteString(lipgloss.NewStyle().Foreground(statusColor(col.name)).Render(strings.Repeat("█", seg)))
		if containsAny(strings.ToLower(col.name), "done", "complete", "closed", "shipped", "merged") {
			doneCount += len(col.items)
		}
	}
	pct := doneCount * 100 / total
	readout := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(fmt.Sprintf("  %d%% done", pct)) +
		detailLabel.Render(fmt.Sprintf(" (%d/%d)", doneCount, total))
	return b.String() + readout
}

func (m Model) renderColumn(idx, colW, bodyH int) string {
	col := m.columns[idx]
	active := idx == m.colCursor
	sc := statusColor(col.name)

	border := paneBorder.BorderForeground(sc)
	if active {
		border = paneBorderActive.BorderForeground(colorAccent)
	}

	innerW := colW - 2
	innerH := bodyH - 2
	perCard := 3
	maxCards := (innerH - 2) / perCard // reserve 2 rows for header
	if maxCards < 1 {
		maxCards = 1
	}

	start := 0
	if active && m.cardCursor >= maxCards {
		start = m.cardCursor - maxCards + 1
	}
	stop := start + maxCards
	if stop > len(col.items) {
		stop = len(col.items)
	}

	var lines []string
	for i := start; i < stop; i++ {
		lines = append(lines, m.renderCard(col.items[i], innerW, active && i == m.cardCursor))
		lines = append(lines, "")
	}
	content := strings.Join(lines, "\n")
	if len(col.items) == 0 {
		content = "\n" + lipgloss.NewStyle().Foreground(colorFaint).Italic(true).Render("   nothing here")
	}
	if stop < len(col.items) {
		content += lipgloss.NewStyle().Foreground(sc).Render(fmt.Sprintf("   ↓ %d more", len(col.items)-stop))
	}

	inner := lipgloss.JoinVertical(lipgloss.Left,
		columnHeaderPill(col.name, len(col.items), innerW, active),
		content,
	)
	return border.Width(innerW).Height(innerH).MarginRight(1).Render(inner)
}

func (m Model) renderCard(it github.ProjectItem, w int, selected bool) string {
	num := "draft"
	if it.Number > 0 {
		num = "#" + itoa(it.Number)
	}
	kind := "○"
	if it.Type == "PULL_REQUEST" {
		kind = "⇄"
	} else if strings.EqualFold(it.State, "CLOSED") {
		kind = "●"
	}
	sc := statusColor(it.Status)

	// Selection is shown by a thick accent bar + bold bright title — no
	// background fill (which fragments behind coloured text in terminals).
	barCh, barCol := "│", sc
	if selected {
		barCh, barCol = "┃", colorAccent
	}
	bar := lipgloss.NewStyle().Foreground(barCol).Render(barCh)

	// Width left for the title after "│ ○ #14 " prefix (bar+space+icon+space+num+space).
	textW := w - 5 - len([]rune(num))
	if textW < 6 {
		textW = 6
	}
	title := truncate(it.Title, textW)
	titleSt := lipgloss.NewStyle().Foreground(colorFg)
	if selected {
		titleSt = titleSt.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	}

	line1 := lipgloss.NewStyle().Foreground(sc).Render(kind) + " " +
		lipgloss.NewStyle().Foreground(colorDim).Render(num) + " " + titleSt.Render(title)

	// Meta line: assignee chips (or "unassigned") + repo, dimmed.
	var meta string
	if len(it.Assignees) > 0 {
		chips := make([]string, 0, len(it.Assignees))
		for _, a := range it.Assignees {
			chips = append(chips, avatarChip(a))
		}
		meta = strings.Join(chips, " ")
	} else {
		meta = lipgloss.NewStyle().Foreground(colorFaint).Render("unassigned")
	}
	if it.RepoName != "" && len(it.Assignees) == 0 {
		// Only show the repo when there's room (no assignee chips crowding it).
		meta += lipgloss.NewStyle().Foreground(colorFaint).Render("  " + truncate(it.RepoName, w-14))
	}
	line2 := "  " + meta

	content := lipgloss.JoinVertical(lipgloss.Left, line1, line2)
	return lipgloss.JoinHorizontal(lipgloss.Top, bar, " ", content)
}

// truncate shortens s to at most w display cells, adding an ellipsis.
func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w < 1 {
		w = 1
	}
	r := []rune(s)
	if len(r) > w-1 {
		r = r[:w-1]
	}
	return string(r) + "…"
}

func (m Model) statusView() string {
	if m.status == "" {
		return statusBar.Render("")
	}
	c := colorFg
	switch m.statKnd {
	case "ok":
		c = colorGreen
	case "err":
		c = colorRed
	}
	return statusBar.Foreground(c).Render(m.status)
}

func (m Model) helpBar() string {
	var pairs [][2]string
	switch m.scr {
	case screenProjects:
		pairs = [][2]string{{"↑↓", "move"}, {"/", "filter"}, {"↵", "open"}, {"o", "web"}, {"?", "help"}, {"X", "logout"}, {"q", "quit"}}
	case screenBoard:
		pairs = [][2]string{{"←→↑↓", "nav"}, {"n", "new"}, {"a", "assign"}, {"s", "status"}, {"p", "field"}, {"c", "comment"}, {"m", "mine"}, {"o", "open"}, {"?", "help"}, {"esc", "back"}}
	case screenAssignee:
		pairs = [][2]string{{"↑↓", "move"}, {"/", "filter"}, {"↵", "toggle"}, {"esc", "back"}}
	case screenStatus:
		pairs = [][2]string{{"↑↓", "move"}, {"↵", "set"}, {"esc", "back"}}
	case screenFieldPick:
		pairs = [][2]string{{"↑↓", "move"}, {"↵", "choose field"}, {"esc", "back"}}
	case screenInput:
		pairs = [][2]string{{"↵", "submit"}, {"esc", "cancel"}}
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, keycap.Render(p[0])+" "+keyLabel.Render(p[1]))
	}
	return "  " + strings.Join(parts, keyLabel.Render("  "))
}

func (m Model) helpOverlay() string {
	rows := [][2]string{
		{"Projects screen", ""},
		{"  ↑ / ↓", "move between boards"},
		{"  /", "filter boards by name"},
		{"  enter", "open the selected board"},
		{"  o", "open board on github.com"},
		{"", ""},
		{"Board (kanban)", ""},
		{"  ← / → (h/l)", "move between columns"},
		{"  ↑ / ↓ (k/j)", "move between cards"},
		{"  n", "create a new draft ticket"},
		{"  a", "assign / unassign the card"},
		{"  s", "move the card to another column"},
		{"  p", "set a field (Priority, Size, …)"},
		{"  c", "add a comment to the ticket"},
		{"  m", "toggle: show only my tickets"},
		{"  o", "open the ticket in your browser"},
		{"  r", "refresh the board"},
		{"  esc / q", "back to projects"},
		{"", ""},
		{"Anywhere", ""},
		{"  ?", "toggle this help"},
		{"  X", "log out (press twice)"},
		{"  ctrl+c", "quit"},
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(" lazyhub — keys ") + "\n\n")
	for _, r := range rows {
		if r[1] == "" {
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(r[0]) + "\n")
			continue
		}
		b.WriteString(fmt.Sprintf("%s  %s\n",
			lipgloss.NewStyle().Foreground(colorFg).Width(16).Render(r[0]),
			detailLabel.Render(r[1])))
	}
	b.WriteString("\n" + helpStyle.Render("  press ? or esc to close"))
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

// --- side effects ---

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Start(); err != nil {
			return actionMsg{text: "open failed: " + err.Error(), kind: "err"}
		}
		return actionMsg{text: "Opened in browser", kind: "ok"}
	}
}
