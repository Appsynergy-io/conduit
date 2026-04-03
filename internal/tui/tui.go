package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Types ────────────────────────────────────────────────────────

type tabID int

const (
	tabAgents   tabID = iota
	tabSessions
)

// Result is returned when the user selects an agent or session.
type Result struct {
	AgentID   string
	AgentName string
	SessionID string // non-empty = resume existing session
}

type model struct {
	client    *Client
	tab       tabID
	agents    []Agent
	sessions  []ShellSession
	cursor    int
	filter    string
	filtering bool
	result    *Result
	width     int
	height    int
	err       error
	loading   bool
}

// ── Messages ─────────────────────────────────────────────────────

type agentsLoadedMsg []Agent
type agentsErrMsg struct{ err error }
type sessionsLoadedMsg []ShellSession

func fetchAgents(client *Client) tea.Cmd {
	return func() tea.Msg {
		agents, err := client.ListAgents(context.Background())
		if err != nil {
			return agentsErrMsg{err}
		}
		return agentsLoadedMsg(agents)
	}
}

func fetchSessions(client *Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := client.ListShellSessions(context.Background())
		if err != nil {
			return sessionsLoadedMsg(nil)
		}
		return sessionsLoadedMsg(sessions)
	}
}

// ── Theme ────────────────────────────────────────────────────────

var (
	colorBrand  = lipgloss.Color("62")
	colorOnline = lipgloss.Color("42")
	colorDim    = lipgloss.Color("241")
	colorMuted  = lipgloss.Color("238")
	colorBright = lipgloss.Color("15")
	colorDetach = lipgloss.Color("214")

	brandStyle       = lipgloss.NewStyle().Bold(true).Foreground(colorBrand)
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorBright).Underline(true)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(colorDim)
	colHeaderStyle   = lipgloss.NewStyle().Foreground(colorDim)
	onlineStyle      = lipgloss.NewStyle().Foreground(colorOnline)
	offlineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	detachedStyle    = lipgloss.NewStyle().Foreground(colorDetach)
	selectedStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle         = lipgloss.NewStyle().Foreground(colorDim)
	mutedStyle       = lipgloss.NewStyle().Foreground(colorMuted)
	keyStyle         = lipgloss.NewStyle().Foreground(colorBright)
	helpTextStyle    = lipgloss.NewStyle().Foreground(colorDim)
)

// ── Program ──────────────────────────────────────────────────────

// Run launches the TUI agent/session browser. Returns the selected target.
func Run(client *Client) (*Result, error) {
	m := model{client: client, loading: true}
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	return final.(model).result, nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchAgents(m.client), fetchSessions(m.client))
}

// ── Update ───────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case agentsLoadedMsg:
		m.agents = []Agent(msg)
		m.loading = false
		return m, nil

	case agentsErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case sessionsLoadedMsg:
		m.sessions = []ShellSession(msg)
		if m.tab == tabSessions && m.cursor >= len(m.visibleSessions()) && len(m.visibleSessions()) > 0 {
			m.cursor = len(m.visibleSessions()) - 1
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNav(msg)
	}
	return m, nil
}

func (m model) updateNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.cursor = 0
		m.filter = ""
		if m.tab == tabAgents {
			m.tab = tabSessions
		} else {
			m.tab = tabAgents
		}

	case "j", "down":
		if m.cursor < m.visibleCount()-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "g", "home":
		m.cursor = 0

	case "G", "end":
		if n := m.visibleCount(); n > 0 {
			m.cursor = n - 1
		}

	case "r":
		m.loading = true
		return m, tea.Batch(fetchAgents(m.client), fetchSessions(m.client))

	case "/":
		m.filtering = true
		m.filter = ""
		m.cursor = 0

	case "enter":
		return m.handleSelect()

	case "p":
		if m.tab == tabSessions {
			return m.handleTogglePin()
		}

	case "x":
		if m.tab == tabSessions {
			return m.handleTerminate()
		}
	}
	return m, nil
}

func (m model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.filtering = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if r := msg.String(); len(r) == 1 {
			m.filter += r
		}
	}
	m.cursor = 0
	return m, nil
}

// ── Actions ──────────────────────────────────────────────────────

func (m model) handleSelect() (tea.Model, tea.Cmd) {
	if m.tab == tabAgents {
		vis := m.visibleAgents()
		if m.cursor < len(vis) && vis[m.cursor].Status == "online" {
			a := vis[m.cursor]
			m.result = &Result{AgentID: a.ID, AgentName: a.Hostname}
			return m, tea.Quit
		}
	} else {
		vis := m.visibleSessions()
		if m.cursor < len(vis) {
			s := vis[m.cursor]
			if s.Status == "active" || s.Status == "detached" {
				m.result = &Result{
					AgentID:   s.AgentID,
					AgentName: m.agentHostname(s.AgentID),
					SessionID: s.ID,
				}
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m model) handleTogglePin() (tea.Model, tea.Cmd) {
	vis := m.visibleSessions()
	if m.cursor >= len(vis) {
		return m, nil
	}
	s := vis[m.cursor]
	client := m.client
	return m, func() tea.Msg {
		client.ToggleSessionPin(s.AgentID, s.ID)
		sessions, _ := client.ListShellSessions(context.Background())
		return sessionsLoadedMsg(sessions)
	}
}

func (m model) handleTerminate() (tea.Model, tea.Cmd) {
	vis := m.visibleSessions()
	if m.cursor >= len(vis) {
		return m, nil
	}
	s := vis[m.cursor]
	client := m.client
	return m, func() tea.Msg {
		client.TerminateSession(s.AgentID, s.ID)
		sessions, _ := client.ListShellSessions(context.Background())
		return sessionsLoadedMsg(sessions)
	}
}

// ── Data Helpers ─────────────────────────────────────────────────

func (m model) visibleAgents() []Agent {
	if m.filter == "" {
		return m.agents
	}
	f := strings.ToLower(m.filter)
	var out []Agent
	for _, a := range m.agents {
		if strings.Contains(strings.ToLower(a.Hostname), f) {
			out = append(out, a)
			continue
		}
		if a.IP != nil && strings.Contains(*a.IP, f) {
			out = append(out, a)
			continue
		}
		if a.OS != nil && strings.Contains(strings.ToLower(*a.OS), f) {
			out = append(out, a)
		}
	}
	return out
}

func (m model) visibleSessions() []ShellSession {
	if m.filter == "" {
		return m.sessions
	}
	f := strings.ToLower(m.filter)
	var out []ShellSession
	for _, s := range m.sessions {
		name := m.agentHostname(s.AgentID)
		if strings.Contains(strings.ToLower(name), f) || strings.Contains(s.Status, f) {
			out = append(out, s)
		}
	}
	return out
}

func (m model) visibleCount() int {
	if m.tab == tabAgents {
		return len(m.visibleAgents())
	}
	return len(m.visibleSessions())
}

func (m model) agentHostname(agentID string) string {
	for _, a := range m.agents {
		if a.ID == agentID {
			if a.DisplayName != nil && *a.DisplayName != "" {
				return *a.DisplayName
			}
			return a.Hostname
		}
	}
	if len(agentID) > 8 {
		return agentID[:8]
	}
	return agentID
}

func (m model) viewport(listLen int) (int, int) {
	available := m.height - 12
	if available < 3 {
		available = 3
	}
	if listLen <= available {
		return 0, listLen
	}
	start := m.cursor - available/2
	if start < 0 {
		start = 0
	}
	end := start + available
	if end > listLen {
		end = listLen
		start = end - available
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

// ── View ─────────────────────────────────────────────────────────

func (m model) View() string {
	w := m.width
	if w < 1 {
		w = 80
	}

	var b strings.Builder

	// Header
	left := brandStyle.Render(" Conduit")
	right := dimStyle.Render(m.client.ServerURL)
	if gap := w - lipgloss.Width(left) - lipgloss.Width(right); gap > 0 {
		b.WriteString(left + strings.Repeat(" ", gap) + right)
	} else {
		b.WriteString(left)
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", w)))
	b.WriteString("\n\n")

	// Loading
	if m.loading {
		b.WriteString("  Loading...\n")
		return b.String()
	}

	// Error
	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n\n", m.err))
		b.WriteString(m.renderHelp())
		return b.String()
	}

	// Tabs + stats
	b.WriteString(m.renderTabs(w))
	b.WriteString("\n\n")

	// Content
	switch m.tab {
	case tabAgents:
		b.WriteString(m.renderAgentList())
	case tabSessions:
		b.WriteString(m.renderSessionList())
	}

	// Filter
	if m.filtering {
		b.WriteString(fmt.Sprintf("\n  / %s▏", m.filter))
	}

	// Help
	b.WriteString("\n")
	b.WriteString(m.renderHelp())
	b.WriteString("\n")

	return b.String()
}

func (m model) renderTabs(w int) string {
	agentLabel := fmt.Sprintf("Agents (%d)", len(m.agents))
	sessLabel := fmt.Sprintf("Sessions (%d)", len(m.sessions))

	var left string
	if m.tab == tabAgents {
		left = "  " + activeTabStyle.Render(agentLabel) + "       " + inactiveTabStyle.Render(sessLabel)
	} else {
		left = "  " + inactiveTabStyle.Render(agentLabel) + "       " + activeTabStyle.Render(sessLabel)
	}

	var stats string
	if m.tab == tabAgents {
		online := 0
		for _, a := range m.agents {
			if a.Status == "online" {
				online++
			}
		}
		stats = dimStyle.Render(fmt.Sprintf("%d online", online))
	} else {
		active, detached := 0, 0
		for _, s := range m.sessions {
			switch s.Status {
			case "active":
				active++
			case "detached":
				detached++
			}
		}
		stats = dimStyle.Render(fmt.Sprintf("%d active · %d detached", active, detached))
	}

	if gap := w - lipgloss.Width(left) - lipgloss.Width(stats) - 2; gap > 0 {
		return left + strings.Repeat(" ", gap) + stats
	}
	return left
}

func (m model) renderAgentList() string {
	var b strings.Builder

	vis := m.visibleAgents()
	if len(vis) == 0 {
		if m.filter != "" {
			b.WriteString("  No agents match filter.\n")
		} else {
			b.WriteString("  No agents registered.\n")
		}
		return b.String()
	}

	// Column header
	b.WriteString("  " + colHeaderStyle.Render(
		pad("  HOSTNAME", 24)+pad("PLATFORM", 16)+pad("ADDRESS", 20)+"STATUS",
	))
	b.WriteString("\n")

	start, end := m.viewport(len(vis))
	if start > 0 {
		b.WriteString(dimStyle.Render("    ↑ more") + "\n")
	}

	for i := start; i < end; i++ {
		a := vis[i]

		hostname := a.Hostname
		if a.DisplayName != nil && *a.DisplayName != "" {
			hostname = *a.DisplayName
		}
		if len(hostname) > 20 {
			hostname = hostname[:17] + "..."
		}

		osArch := ""
		if a.OS != nil {
			osArch = *a.OS
		}
		if a.Arch != nil {
			osArch += "/" + *a.Arch
		}

		ip := ""
		if a.IP != nil {
			ip = *a.IP
		}

		var status string
		if a.Status == "online" {
			status = onlineStyle.Render("● online")
		} else {
			status = offlineStyle.Render("○ offline")
		}

		row := pad(hostname, 22) + pad(osArch, 16) + pad(ip, 20) + status
		if i == m.cursor {
			b.WriteString(brandStyle.Render("  ▸ ") + selectedStyle.Render(row) + "\n")
		} else {
			b.WriteString("    " + row + "\n")
		}
	}

	if end < len(vis) {
		b.WriteString(dimStyle.Render("    ↓ more") + "\n")
	}

	return b.String()
}

func (m model) renderSessionList() string {
	var b strings.Builder

	vis := m.visibleSessions()
	if len(vis) == 0 {
		b.WriteString("  No active sessions.\n")
		return b.String()
	}

	// Column header
	b.WriteString("  " + colHeaderStyle.Render(
		pad("  AGENT", 24)+pad("STATUS", 16)+pad("PIN", 10)+"STARTED",
	))
	b.WriteString("\n")

	start, end := m.viewport(len(vis))
	if start > 0 {
		b.WriteString(dimStyle.Render("    ↑ more") + "\n")
	}

	for i := start; i < end; i++ {
		s := vis[i]

		agentName := m.agentHostname(s.AgentID)
		if len(agentName) > 20 {
			agentName = agentName[:17] + "..."
		}

		var status string
		switch s.Status {
		case "active":
			status = onlineStyle.Render("● active")
		case "detached":
			status = detachedStyle.Render("◌ detached")
		default:
			status = offlineStyle.Render("○ " + s.Status)
		}

		pin := ""
		if s.Pinned == 1 {
			pin = brandStyle.Render("◆")
		}

		started := relativeTime(s.CreatedAt)

		row := pad(agentName, 22) + pad(status, 16) + pad(pin, 10) + started
		if i == m.cursor {
			b.WriteString(brandStyle.Render("  ▸ ") + selectedStyle.Render(row) + "\n")
		} else {
			b.WriteString("    " + row + "\n")
		}
	}

	if end < len(vis) {
		b.WriteString(dimStyle.Render("    ↓ more") + "\n")
	}

	return b.String()
}

func (m model) renderHelp() string {
	k := func(key, desc string) string {
		return keyStyle.Render(key) + helpTextStyle.Render(" "+desc)
	}

	parts := []string{k("↑↓", "navigate")}

	if m.tab == tabAgents {
		parts = append(parts, k("enter", "connect"), k("/", "filter"), k("tab", "sessions"))
	} else {
		parts = append(parts, k("enter", "resume"), k("p", "pin"), k("x", "close"), k("/", "filter"), k("tab", "agents"))
	}

	parts = append(parts, k("r", "refresh"), k("q", "quit"))

	return "  " + strings.Join(parts, "  ")
}

// ── Utilities ────────────────────────────────────────────────────

// pad right-pads a string to the given visual width, accounting for ANSI codes.
func pad(s string, width int) string {
	vw := lipgloss.Width(s)
	if vw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vw)
}

func relativeTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z07:00", timestamp)
		if err != nil {
			return timestamp
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
