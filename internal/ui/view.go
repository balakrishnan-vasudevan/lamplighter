package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
)

var colColors = []lipgloss.Color{
	"#58a6ff",
	"#3fb950",
	"#d29922",
	"#f78166",
	"#bc8cff",
	"#39d353",
}

var (
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e7681"))
	styleError = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922"))
)

func colColor(idx int) lipgloss.Color {
	return colColors[idx%len(colColors)]
}

func (m *Model) View() string {
	if m.showHelp {
		return helpView()
	}

	var content string
	if m.focused && len(m.columns) > 0 {
		content = m.singleView(m.cursor)
	} else {
		content = m.multiView()
	}

	return content + "\n" + m.statusBar()
}

func (m *Model) multiView() string {
	n := len(m.columns)
	if n == 0 || m.width == 0 || m.height == 0 {
		return ""
	}

	colWidth := (m.width - (n - 1)) / n
	contentRows := m.contentRows()

	colRows := make([][]string, n)
	for i, col := range m.columns {
		colRows[i] = m.buildColumnRows(i, col, colWidth, contentRows)
	}

	totalRows := len(colRows[0])
	sep := styleDim.Render("│")

	var sb strings.Builder
	for row := 0; row < totalRows; row++ {
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteString(sep)
			}
			if row < len(colRows[i]) {
				sb.WriteString(colRows[i][row])
			} else {
				sb.WriteString(strings.Repeat(" ", colWidth))
			}
		}
		if row < totalRows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func (m *Model) singleView(idx int) string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	rows := m.buildColumnRows(idx, m.columns[idx], m.width, m.contentRows())
	return strings.Join(rows, "\n")
}

func (m *Model) buildColumnRows(idx int, col *column.Column, colWidth, contentRows int) []string {
	color := colColor(idx)
	isCursor := idx == m.cursor

	rawStatus, _ := statusIcon(col.GetStatus())
	rawHeader := rawStatus + " " + col.Header()
	if m.paused[idx] {
		rawHeader += " [PAUSED]"
	}
	if isCursor && !m.focused {
		rawHeader += " ◀"
	}
	rawHeader = truncatePad(rawHeader, colWidth)

	var headerStyle lipgloss.Style
	if isCursor && !m.focused {
		headerStyle = lipgloss.NewStyle().Foreground(color).Bold(true).Underline(true)
	} else {
		headerStyle = lipgloss.NewStyle().Foreground(color).Bold(true)
	}

	rows := []string{
		headerStyle.Render(rawHeader),
		styleDim.Render(strings.Repeat("─", colWidth)),
	}

	snapshot := m.snapshots[idx]
	for len(snapshot) < contentRows {
		snapshot = append([]buffer.Line{{}}, snapshot...)
	}
	for _, line := range snapshot {
		rows = append(rows, m.renderLine(line, colWidth))
	}
	return rows
}

func (m *Model) renderLine(line buffer.Line, colWidth int) string {
	if line.Text == "" && line.Timestamp.IsZero() {
		return strings.Repeat(" ", colWidth)
	}

	const tsLen = 9 // "15:04:05 "
	textWidth := colWidth - tsLen

	if textWidth < 4 {
		raw := truncatePad(line.Text, colWidth)
		return m.applySearch(raw, line)
	}

	rawTS := strings.Repeat(" ", tsLen)
	if !line.Timestamp.IsZero() {
		rawTS = line.Timestamp.Format("15:04:05 ")
	}
	displayText := line.Text
	if len(line.Extra) > 0 {
		displayText += fmt.Sprintf(" (+%d)", len(line.Extra))
	}
	rawText := truncatePad(displayText, textWidth)

	// Dim non-matching lines when a search is active.
	if m.searchRegex != nil && !lineMatchesSearch(m.searchRegex, line) {
		return styleDim.Render(rawTS + rawText)
	}

	return styleDim.Render(rawTS) + applyLevel(rawText, line.Level)
}

func (m *Model) applySearch(raw string, line buffer.Line) string {
	if m.searchRegex != nil && !lineMatchesSearch(m.searchRegex, line) {
		return styleDim.Render(raw)
	}
	return applyLevel(raw, line.Level)
}

func lineMatchesSearch(re *regexp.Regexp, line buffer.Line) bool {
	if re.MatchString(line.Text) {
		return true
	}
	if line.RawText != "" && re.MatchString(line.RawText) {
		return true
	}
	for _, v := range line.Fields {
		if re.MatchString(v) {
			return true
		}
	}
	for _, extra := range line.Extra {
		if re.MatchString(extra) {
			return true
		}
	}
	return false
}

func applyLevel(s string, level buffer.LogLevel) string {
	switch level {
	case buffer.LevelError:
		return styleError.Render(s)
	case buffer.LevelWarn:
		return styleWarn.Render(s)
	}
	return s
}

func (m *Model) statusBar() string {
	// Left: mode indicator
	var leftText string
	var leftStyle lipgloss.Style
	if m.liveMode {
		leftText = "[LIVE]"
		leftStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	} else {
		leftText = "[" + m.viewTime.UTC().Format("2006-01-02 15:04:05") + "]  ↑/↓ scroll · G live"
		leftStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922")).Bold(true)
	}

	// Right: search indicator
	var rightText string
	var rightStyle lipgloss.Style
	if m.searchMode {
		rightText = "/ " + m.searchQuery + "█"
		rightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	} else if m.searchRegex != nil {
		rightText = "/ " + m.searchQuery + "  Esc clear"
		rightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	} else {
		rightText = "/ search  ? help  q quit"
		rightStyle = styleDim
	}

	pad := m.width - len([]rune(leftText)) - len([]rune(rightText))
	if pad < 1 {
		pad = 1
	}

	return leftStyle.Render(leftText) + strings.Repeat(" ", pad) + rightStyle.Render(rightText)
}

func statusIcon(s column.Status) (raw, styled string) {
	switch s {
	case column.Streaming:
		raw = "●"
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Render(raw)
	case column.Reconnecting:
		raw = "○"
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922")).Render(raw)
	case column.Dead:
		raw = "✕"
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Render(raw)
	default:
		raw, styled = "●", "●"
	}
	return
}

func helpView() string {
	lines := []string{
		"  Lamplighter - Keyboard Controls",
		"",
		"  Tab / →     next column",
		"  ←           previous column",
		"  p           pause / unpause column",
		"  f           focus column full-screen",
		"  r           reconnect column",
		"",
		"  ↑           scroll back (all columns move together)",
		"  ↓           scroll forward",
		"  g           jump to oldest line in buffer",
		"  G           return to live",
		"",
		"  /           open search - type a regex or string",
		"              matching lines stay bright, rest dims",
		"  Enter       lock search and close input",
		"  Esc         clear search",
		"",
		"  ?           toggle this help",
		"  q / Ctrl+C  quit",
	}
	return strings.Join(lines, "\n")
}

func truncatePad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) > width {
		if width == 1 {
			return "…"
		}
		return string(runes[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// Unused import guard
var _ = time.Now
