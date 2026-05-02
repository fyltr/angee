package dev

import (
	"bufio"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PaneSink is the bubbletea-backed Sink for `angee dev --ui=panes`.
//
// One scrollable viewport per child + an "all" tab. Tab/Shift-Tab to
// cycle, `q` or Ctrl-C to quit (sends SIGINT to the orchestrator).
//
// Each Writer() returns an io.Pipe whose reader feeds a bubbletea
// goroutine; we never block child stdio. SystemLine emits go-routine-safe
// updates via tea.Program.Send.
type PaneSink struct {
	prog    *tea.Program
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	startMu sync.Mutex
	started bool
}

// NewPaneSink initialises the sink and starts the bubbletea program in a
// goroutine. Caller must invoke Wait() in the main thread before exit so
// the terminal is restored cleanly.
func NewPaneSink() *PaneSink {
	model := newPanesModel()
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	ps := &PaneSink{prog: prog}
	_, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	ps.wg.Add(1)
	go func() {
		defer ps.wg.Done()
		_, _ = prog.Run()
		cancel()
	}()
	return ps
}

// Writer returns a per-child io.Writer; each line written becomes a
// row in that child's viewport.
func (ps *PaneSink) Writer(name string) io.Writer {
	pr, pw := io.Pipe()
	ps.markStarted(name)
	go func() {
		defer pr.Close()
		scan := bufio.NewScanner(pr)
		scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scan.Scan() {
			ps.prog.Send(appendLineMsg{name: name, line: scan.Text()})
		}
	}()
	return pw
}

// SystemLine sends an orchestrator-level message into the [angee] tab.
func (ps *PaneSink) SystemLine(format string, args ...any) {
	ps.markStarted("angee")
	ps.prog.Send(appendLineMsg{
		name: "angee",
		line: fmt.Sprintf(format, args...),
	})
}

// Wait blocks until the bubbletea program quits (typically via `q` /
// Ctrl-C). Restores the terminal alt-screen state on the way out.
func (ps *PaneSink) Wait() {
	ps.wg.Wait()
}

// Quit asks the TUI to shut down. Used when the orchestrator returns.
func (ps *PaneSink) Quit() {
	ps.prog.Quit()
}

// Done returns a channel that closes when the user dismisses the TUI
// (q / ctrl-c). The dev orchestrator listens on this and treats it as
// SIGINT so children get a chance to clean up.
func (ps *PaneSink) Done() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		ps.wg.Wait()
		close(done)
	}()
	return done
}

func (ps *PaneSink) markStarted(name string) {
	ps.startMu.Lock()
	defer ps.startMu.Unlock()
	ps.started = true
	ps.prog.Send(ensureTabMsg{name: name})
}

// ── bubbletea model ────────────────────────────────────────────────────

type panesModel struct {
	tabs    []string                 // ordered tab labels
	lines   map[string][]string      // per-tab buffered lines
	views   map[string]viewport.Model // per-tab viewport (lazy)
	active  int                       // active tab index
	width   int
	height  int
	ready   bool
}

func newPanesModel() panesModel {
	return panesModel{
		tabs:  []string{"all"},
		lines: map[string][]string{"all": {}},
		views: map[string]viewport.Model{},
	}
}

// Messages.

type appendLineMsg struct{ name, line string }
type ensureTabMsg struct{ name string }

func (m panesModel) Init() tea.Cmd { return nil }

func (m panesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "right", "l":
			m.active = (m.active + 1) % len(m.tabs)
		case "shift+tab", "left", "h":
			m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Resize all viewports.
		for name, v := range m.views {
			v.Width = msg.Width
			v.Height = m.contentHeight()
			m.views[name] = v
		}
	case ensureTabMsg:
		if !contains(m.tabs, msg.name) {
			m.tabs = append(m.tabs, msg.name)
			m.lines[msg.name] = nil
		}
	case appendLineMsg:
		// Append to per-tab buffer + the synthetic "all" buffer.
		stamped := fmt.Sprintf("[%s] %s", msg.name, msg.line)
		m.lines["all"] = append(m.lines["all"], stamped)
		m.lines[msg.name] = append(m.lines[msg.name], msg.line)
		m.setViewportContent(msg.name)
		m.setViewportContent("all")
	}
	// Forward key/scroll events to the active viewport.
	if m.ready {
		var cmd tea.Cmd
		v := m.activeViewport()
		v, cmd = v.Update(msg)
		m.views[m.tabs[m.active]] = v
		return m, cmd
	}
	return m, nil
}

func (m panesModel) View() string {
	if !m.ready {
		return "starting…"
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderTabBar(),
		m.activeViewport().View(),
		m.renderStatusBar(),
	)
}

// ── view helpers ───────────────────────────────────────────────────────

func (m panesModel) renderTabBar() string {
	var parts []string
	for i, name := range m.tabs {
		st := tabStyle
		if i == m.active {
			st = activeTabStyle.Foreground(colorFor(name))
		} else {
			st = tabStyle.Foreground(colorFor(name))
		}
		parts = append(parts, st.Render(name))
	}
	return tabBarStyle.Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, parts...),
	)
}

func (m panesModel) renderStatusBar() string {
	right := "tab/shift-tab: switch · q/ctrl-c: quit"
	return statusBarStyle.Width(m.width).Render(right)
}

func (m panesModel) contentHeight() int {
	// 1 line tab bar + 1 line status bar.
	return m.height - 2
}

func (m *panesModel) setViewportContent(name string) {
	v, ok := m.views[name]
	if !ok {
		v = viewport.New(m.width, m.contentHeight())
	}
	body := strings.Join(m.lines[name], "\n")
	v.SetContent(body)
	v.GotoBottom()
	m.views[name] = v
}

func (m *panesModel) activeViewport() viewport.Model {
	if m.active < 0 || m.active >= len(m.tabs) {
		return viewport.Model{}
	}
	name := m.tabs[m.active]
	v, ok := m.views[name]
	if !ok {
		v = viewport.New(m.width, m.contentHeight())
		v.SetContent(strings.Join(m.lines[name], "\n"))
		m.views[name] = v
	}
	return v
}

// colorFor mirrors output_lines.go's stable per-name colour: hash the
// name and pick from a small palette. Returns a lipgloss color.
func colorFor(name string) lipgloss.Color {
	pal := []string{"#7DD3FC", "#FCD34D", "#F0ABFC", "#86EFAC", "#93C5FD", "#FCA5A5", "#E5E7EB", "#9CA3AF"}
	i := int(crc32.ChecksumIEEE([]byte(name))) % len(pal)
	return lipgloss.Color(pal[i])
}

// ── styles ────────────────────────────────────────────────────────────

var (
	tabBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1)

	tabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Bold(false)

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true).
			Underline(true)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#111827")).
			Foreground(lipgloss.Color("#9CA3AF")).
			Padding(0, 1)
)

// stdoutIsTTY is exposed so callers can decide whether panes mode is
// safe (it requires a real TTY for alt-screen).
func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
