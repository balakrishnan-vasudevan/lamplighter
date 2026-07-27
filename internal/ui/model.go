package ui

import (
	"regexp"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/stream"
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type Model struct {
	columns   []*column.Column
	manager   *stream.Manager
	width     int
	height    int
	cursor    int
	focused   bool
	paused    []bool
	showHelp  bool
	snapshots [][]buffer.Line

	// scroll / timestamp alignment
	liveMode bool
	viewTime time.Time

	// search / correlation
	searchMode  bool
	searchQuery string
	searchRegex *regexp.Regexp
}

func New(cols []*column.Column, mgr *stream.Manager) *Model {
	return &Model{
		columns:   cols,
		manager:   mgr,
		paused:    make([]bool, len(cols)),
		snapshots: make([][]buffer.Line, len(cols)),
		liveMode:  true,
	}
}

func (m *Model) Init() tea.Cmd {
	return tick()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.refreshSnapshots()
		return m, tick()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Search input mode consumes all keys except Enter/Esc.
	if m.searchMode {
		switch msg.String() {
		case keyEnter:
			m.searchMode = false
		case keyEsc:
			m.searchMode = false
			m.searchQuery = ""
			m.searchRegex = nil
		case keyBackspace, keyBackspace2:
			if len(m.searchQuery) > 0 {
				runes := []rune(m.searchQuery)
				m.searchQuery = string(runes[:len(runes)-1])
				m.updateSearchRegex()
			}
		default:
			r := []rune(msg.String())
			if len(r) == 1 && r[0] >= 32 {
				m.searchQuery += string(r)
				m.updateSearchRegex()
			}
		}
		return m, nil
	}

	switch msg.String() {
	case keyQuit, keyCtrlC:
		return m, tea.Quit

	case keyHelp:
		m.showHelp = !m.showHelp

	case keySearch:
		m.searchMode = true

	case keyEsc:
		// Clear active search.
		m.searchQuery = ""
		m.searchRegex = nil

	case keyTab, keyRight:
		if len(m.columns) > 0 {
			m.cursor = (m.cursor + 1) % len(m.columns)
		}

	case keyLeft:
		if len(m.columns) > 0 {
			m.cursor = (m.cursor - 1 + len(m.columns)) % len(m.columns)
		}

	case keyPause:
		if len(m.columns) > 0 {
			m.paused[m.cursor] = !m.paused[m.cursor]
		}

	case keyFocus:
		m.focused = !m.focused

	case keyReconnect:
		if len(m.columns) > 0 {
			m.manager.Reconnect(m.columns[m.cursor])
		}

	case keyUp:
		if m.liveMode {
			// First press: anchor to current oldest visible timestamp.
			m.liveMode = false
			m.viewTime = m.oldestVisible()
			if m.viewTime.IsZero() {
				m.viewTime = time.Now()
			}
		} else {
			m.viewTime = m.viewTime.Add(-m.visibleSpan())
		}

	case keyDown:
		if !m.liveMode {
			m.viewTime = m.viewTime.Add(m.visibleSpan())
			if time.Since(m.viewTime) < 2*time.Second {
				m.liveMode = true
				m.viewTime = time.Time{}
			}
		}

	case keyGotoOldest:
		m.liveMode = false
		m.viewTime = m.absoluteOldest()

	case keyGotoLive:
		m.liveMode = true
		m.viewTime = time.Time{}
	}

	return m, nil
}

func (m *Model) updateSearchRegex() {
	if m.searchQuery == "" {
		m.searchRegex = nil
		return
	}
	re, err := regexp.Compile(m.searchQuery)
	if err == nil {
		m.searchRegex = re
	}
}

func (m *Model) refreshSnapshots() {
	contentRows := m.contentRows()
	for i, col := range m.columns {
		if m.paused[i] {
			continue
		}
		if m.liveMode {
			m.snapshots[i] = col.Buffer.Read(contentRows)
		} else {
			m.snapshots[i] = col.Buffer.ReadBefore(m.viewTime, contentRows)
		}
	}
}

func (m *Model) contentRows() int {
	n := m.height - 4 // header + separator + status bar + 1 padding
	if n < 1 {
		return 1
	}
	return n
}

// visibleSpan returns the time range currently visible across all snapshots.
// Used as the scroll step size so one keypress = one page.
func (m *Model) visibleSpan() time.Duration {
	var oldest, newest time.Time
	for _, snap := range m.snapshots {
		for _, l := range snap {
			if l.Timestamp.IsZero() {
				continue
			}
			if oldest.IsZero() || l.Timestamp.Before(oldest) {
				oldest = l.Timestamp
			}
			if newest.IsZero() || l.Timestamp.After(newest) {
				newest = l.Timestamp
			}
		}
	}
	if oldest.IsZero() || newest.IsZero() {
		return time.Minute
	}
	d := newest.Sub(oldest)
	if d < time.Second {
		return time.Second
	}
	return d
}

func (m *Model) oldestVisible() time.Time {
	var oldest time.Time
	for _, snap := range m.snapshots {
		for _, l := range snap {
			if l.Timestamp.IsZero() {
				continue
			}
			if oldest.IsZero() || l.Timestamp.Before(oldest) {
				oldest = l.Timestamp
			}
		}
	}
	return oldest
}

func (m *Model) absoluteOldest() time.Time {
	var oldest time.Time
	for _, col := range m.columns {
		lines := col.Buffer.Read(col.Buffer.Len())
		for _, l := range lines {
			if l.Timestamp.IsZero() {
				continue
			}
			if oldest.IsZero() || l.Timestamp.Before(oldest) {
				oldest = l.Timestamp
			}
		}
	}
	if oldest.IsZero() {
		return time.Now()
	}
	return oldest
}
