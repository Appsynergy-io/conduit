package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Result is returned when the user selects an agent for a shell session.
type Result struct {
	AgentID   string
	AgentName string
}

type model struct {
	client    *Client
	agents    []Agent
	cursor    int
	filter    string
	filtering bool
	result    *Result
	width     int
	height    int
	err       error
	loading   bool
}

type agentsLoadedMsg []Agent
type agentsErrMsg struct{ err error }

func fetchAgents(client *Client) tea.Cmd {
	return func() tea.Msg {
		agents, err := client.ListAgents(context.Background())
		if err != nil {
			return agentsErrMsg{err}
		}
		return agentsLoadedMsg(agents)
	}
}

// Run launches the TUI agent list. Returns the selected agent, or nil if quit.
func Run(client *Client) (*Result, error) {
	m := model{
		client:  client,
		loading: true,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	return final.(model).result, nil
}

func (m model) Init() tea.Cmd {
	return fetchAgents(m.client)
}

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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	vis := m.visible()

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.cursor < len(vis)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "g", "home":
		m.cursor = 0

	case "G", "end":
		if len(vis) > 0 {
			m.cursor = len(vis) - 1
		}

	case "r":
		m.loading = true
		return m, fetchAgents(m.client)

	case "/":
		m.filtering = true
		m.filter = ""
		m.cursor = 0

	case "enter":
		if len(vis) > 0 && m.cursor < len(vis) {
			a := vis[m.cursor]
			if a.Status == "online" {
				m.result = &Result{AgentID: a.ID, AgentName: a.Hostname}
				return m, tea.Quit
			}
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
		r := msg.String()
		if len(r) == 1 {
			m.filter += r
		}
	}
	m.cursor = 0
	return m, nil
}

func (m model) visible() []Agent {
	if m.filter == "" {
		return m.agents
	}
	f := strings.ToLower(m.filter)
	var out []Agent
	for _, a := range m.agents {
		match := strings.Contains(strings.ToLower(a.Hostname), f)
		if !match && a.IP != nil {
			match = strings.Contains(*a.IP, f)
		}
		if !match && a.OS != nil {
			match = strings.Contains(strings.ToLower(*a.OS), f)
		}
		if match {
			out = append(out, a)
		}
	}
	return out
}

// ── View ─────────────────────────────────────────────────────────────────

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	onlineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	offlineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
)

func (m model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Conduit"))
	b.WriteString("  ")
	b.WriteString(dimStyle.Render(m.client.ServerURL))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString("  Loading agents...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", m.err))
		b.WriteString(helpStyle.Render("  r: retry  q: quit"))
		return b.String()
	}

	vis := m.visible()

	// Count online
	online := 0
	for _, a := range m.agents {
		if a.Status == "online" {
			online++
		}
	}
	b.WriteString(fmt.Sprintf("  %d agents (%s%d online%s)\n\n",
		len(m.agents),
		"\033[32m", online, "\033[0m"))

	if len(vis) == 0 {
		if m.filter != "" {
			b.WriteString("  No agents match filter.\n")
		} else {
			b.WriteString("  No agents registered.\n")
		}
	}

	// Header
	b.WriteString(dimStyle.Render(fmt.Sprintf("    %-22s %-14s %-18s %s", "HOSTNAME", "OS/ARCH", "IP", "STATUS")))
	b.WriteString("\n")

	for i, a := range vis {
		prefix := "  "
		if i == m.cursor {
			prefix = "▸ "
		}

		status := offlineStyle.Render("○ offline")
		if a.Status == "online" {
			status = onlineStyle.Render("● online")
		}

		hostname := a.Hostname
		if a.DisplayName != nil && *a.DisplayName != "" {
			hostname = *a.DisplayName
		}
		if len(hostname) > 20 {
			hostname = hostname[:20]
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

		line := fmt.Sprintf("%s%-22s %-14s %-18s %s", prefix, hostname, osArch, ip, status)
		if i == m.cursor {
			line = cursorStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	if m.filtering {
		b.WriteString(fmt.Sprintf("\n  / %s▏", m.filter))
	}

	b.WriteString(helpStyle.Render("  j/k: navigate  Enter: shell  /: search  r: refresh  q: quit"))
	b.WriteString("\n")

	return b.String()
}
