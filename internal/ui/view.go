package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Hxb8/Termdoro/internal/app"
)

// themeColor maps the app theme to a terminal color.
func themeColor(t app.Theme) lipgloss.TerminalColor {
	switch t {
	case app.ThemeMagenta:
		return lipgloss.Color("5")
	case app.ThemeGreen:
		return lipgloss.Color("2")
	case app.ThemeYellow:
		return lipgloss.Color("3")
	case app.ThemeRed:
		return lipgloss.Color("1")
	default:
		return lipgloss.Color("6")
	}
}

// View renders the whole terminal, mirroring the Ratatui layout: a header, a
// centered main panel and a help footer.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	a := m.app
	theme := themeColor(a.Theme)
	width := m.width
	if width < 20 {
		width = 80
	}
	boxWidth := width * 70 / 100
	if boxWidth < 30 {
		boxWidth = 30
	}
	if boxWidth > width-2 {
		boxWidth = width - 2
	}

	header := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme).
		Width(boxWidth).
		Align(lipgloss.Center).
		Render("POMODORO TUI")

	var body string
	var title string
	switch {
	case m.downloading:
		title = " Background Process "
		body = "\n\n" + m.downloadMsg
	default:
		title, body = m.screenView(boxWidth)
	}

	main := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme).
		Width(boxWidth).
		Render(titleBlock(title) + body)

	footer := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Width(boxWidth).
		Align(lipgloss.Center).
		Render(helpText(a.Screen, m.downloading))

	page := lipgloss.JoinVertical(lipgloss.Center, header, main, footer)
	return lipgloss.Place(width, m.height, lipgloss.Center, lipgloss.Center, page)
}

func titleBlock(title string) string {
	if title == "" {
		return ""
	}
	return title + "\n"
}

// screenView returns the panel title and body for the active screen.
func (m *Model) screenView(boxWidth int) (string, string) {
	a := m.app
	switch a.Screen {
	case app.ScreenResume:
		info := "\n\n⏸️ Previous session found!\n\n"
		if s := a.SavedTimerSummary(); s.Found {
			info += fmt.Sprintf("%s | %s | Session %d of %d\n%d:%02d remaining",
				s.ActName, s.Phase, s.Current, s.Total, s.RemMins, s.RemSecs)
		} else {
			info += "Could not read session details."
		}
		return " Resume Session? ", centerText(info)
	case app.ScreenActivity:
		items := append([]string{}, a.Acts...)
		items = append(items, "Previous Sessions 📋", "Session Stats 📊")
		return " [1] Select Activity ", renderList(items, a.Idx, m.theme())
	case app.ScreenAddActivity:
		return " Add Custom Activity ", centerText(fmt.Sprintf("\nActivity Name:\n%s\n\n[Enter] Add | [Esc] Cancel", a.Input))
	case app.ScreenRenameActivity:
		return " Rename Activity ", centerText(fmt.Sprintf("\nNew Name:\n%s\n\n[Enter] Save | [Esc] Cancel\nAll history entries will be updated.", a.Input))
	case app.ScreenDuration:
		return " [2] Duration ", centerText(fmt.Sprintf("\n\nFocus Time: %d min\n\n[J/K] Adjust | [L/Right] Next", a.Mins))
	case app.ScreenSessions:
		return " [3] Sessions ", centerText(fmt.Sprintf("\n\nSessions: %d\n\n[J/K] Adjust | [L/Right] Next", a.Total))
	case app.ScreenBGM:
		return " [4] Background Song (Press 'i' to Import) ", renderList(a.BGMList, a.BGMIdx, m.theme())
	case app.ScreenBGMImport:
		return " Import BGM ", centerText(fmt.Sprintf("\nPaste YouTube URL:\n%s\n\n[Enter] Download | [Esc] Cancel", a.Input))
	case app.ScreenSettings:
		return " Settings ", m.settingsView()
	case app.ScreenTimer:
		return fmt.Sprintf(" Session %d of %d ", a.Current, a.Total), m.timerView(boxWidth)
	case app.ScreenHistory:
		history := a.LoadHistoryDisplay()
		if len(history) == 0 {
			history = []string{"No sessions recorded yet."}
		}
		a.ClampHistoryScroll()
		return " [5] Previous Sessions ", renderList(history, a.HistoryScroll, m.theme())
	case app.ScreenSessionForm:
		if a.FormEditIdx != nil {
			return " Edit Session ", m.formView()
		}
		return " Add Session ", m.formView()
	case app.ScreenStats:
		stats := a.LoadStats()
		if len(stats) == 0 {
			return " [6] Session Stats ", centerText("\n\nNo session data yet.")
		}
		items := make([]string, 0, len(stats))
		for _, s := range stats {
			var timeStr string
			if h, mm := s.TotalMinutes/60, s.TotalMinutes%60; h > 0 {
				timeStr = fmt.Sprintf("%dh %dmin", h, mm)
			} else {
				timeStr = fmt.Sprintf("%dmin", s.TotalMinutes)
			}
			var avg float64
			if s.TotalEntries > 0 {
				avg = float64(s.TotalSessions) / float64(s.TotalEntries)
			}
			items = append(items, fmt.Sprintf("%s | %d sessions (%d entries, %.1f avg) | %s | %d done, %d quit",
				s.Name, s.TotalSessions, s.TotalEntries, avg, timeStr, s.Completed, s.Abandoned))
		}
		a.ClampStatsScroll()
		return " [6] Session Stats ", renderList(items, a.StatsScroll, m.theme())
	default:
		return "", ""
	}
}

func (m *Model) theme() lipgloss.TerminalColor { return themeColor(m.app.Theme) }

// renderList renders selectable rows with the active row highlighted.
func renderList(items []string, selected int, theme lipgloss.TerminalColor) string {
	hl := lipgloss.NewStyle().Background(theme).Foreground(lipgloss.Color("0"))
	var b strings.Builder
	for i, item := range items {
		if i == selected {
			b.WriteString(hl.Render(item))
		} else {
			b.WriteString(item)
		}
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func centerText(s string) string {
	lines := strings.Split(s, "\n")
	width := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > width {
			width = w
		}
	}
	var b strings.Builder
	for i, l := range lines {
		pad := (width - lipgloss.Width(l)) / 2
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(l)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m *Model) settingsView() string {
	a := m.app
	theme := m.theme()
	nStatus := "OFF"
	if a.NotificationsEnabled {
		nStatus = "ON"
	}
	rows := []struct {
		label  string
		value  string
		active bool
	}{
		{"Notifications: ", nStatus, a.SettingsCursor == 0},
		{"Theme: ", a.Theme.String(), a.SettingsCursor == 1},
		{"Break Duration: ", fmt.Sprintf("%d min", a.BreakMins), a.SettingsCursor == 2},
	}
	var b strings.Builder
	for i, r := range rows {
		prefix := "  "
		if r.active {
			prefix = lipgloss.NewStyle().Foreground(theme).Render("> ")
		}
		name := r.label
		if r.active {
			name = lipgloss.NewStyle().Foreground(theme).Render(r.label)
		}
		b.WriteString(prefix + name + r.value)
		if i < len(rows)-1 {
			b.WriteString("\n\n")
		}
	}
	return centerText(b.String())
}

// timerView renders the progress bar with the white-bold countdown label.
func (m *Model) timerView(boxWidth int) string {
	a := m.app
	var total uint32
	if a.Work {
		total = a.Mins * 60
	} else {
		total = a.BreakMins * 60
	}
	var pct float64
	if total > 0 {
		done := total - minU32(a.Rem, total)
		pct = float64(done) / float64(total) * 100
	}
	if pct > 100 {
		pct = 100
	}
	var gauge lipgloss.TerminalColor
	switch {
	case a.Paused:
		gauge = lipgloss.Color("8")
	case a.Work:
		gauge = lipgloss.Color("1")
	default:
		gauge = lipgloss.Color("2")
	}
	var phase string
	switch {
	case a.Paused:
		phase = "⏸ PAUSED"
	case a.Work:
		phase = "🔥 FOCUS"
	default:
		phase = "☕ BREAK"
	}
	var vol string
	if a.Muted {
		vol = "Muted"
	} else {
		vol = fmt.Sprintf("%d%%", uint32(a.Volume*100))
	}
	label := fmt.Sprintf("%s | %d:%02d | Vol: %s", phase, a.Rem/60, a.Rem%60, vol)

	barWidth := boxWidth - 6
	if barWidth < 10 {
		barWidth = 10
	}
	filled := int(pct / 100 * float64(barWidth))
	labelRunes := []rune(label)
	start := (barWidth - len(labelRunes)) / 2
	if start < 0 {
		start = 0
	}
	fillStyle := lipgloss.NewStyle().Foreground(gauge)
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	var b strings.Builder
	for i := 0; i < barWidth; i++ {
		switch {
		case i >= start && i < start+len(labelRunes):
			var bg lipgloss.TerminalColor = lipgloss.Color("8")
			if i < filled {
				bg = gauge
			}
			b.WriteString(labelStyle.Background(bg).Render(string(labelRunes[i-start])))
		case i < filled:
			b.WriteString(fillStyle.Render("█"))
		default:
			b.WriteString(emptyStyle.Render("░"))
		}
	}
	return b.String()
}

func (m *Model) formView() string {
	a := m.app
	theme := m.theme()
	actName := "?"
	if a.FormActIdx >= 0 && a.FormActIdx < len(a.Acts) {
		actName = a.Acts[a.FormActIdx]
	}
	completed := "No"
	if a.FormCompleted {
		completed = "Yes"
	}
	fields := [][2]string{
		{"Date", app.FormatDateDisplay(a.FormDate)},
		{"Activity", fmt.Sprintf("< %s >", actName)},
		{"Duration", fmt.Sprintf("< %d min >", a.FormMins)},
		{"Break", fmt.Sprintf("< %d min >", a.FormBreakMins)},
		{"Sessions Done", fmt.Sprintf("< %d >", a.FormDone)},
		{"Sessions Planned", fmt.Sprintf("< %d >", a.FormPlanned)},
		{"Completed", fmt.Sprintf("< %s >", completed)},
	}
	var b strings.Builder
	for i, f := range fields {
		prefix := "  "
		if i == a.FormCursor {
			prefix = lipgloss.NewStyle().Foreground(theme).Render("> ")
		}
		label := f[0] + ": "
		if i == a.FormCursor {
			label = lipgloss.NewStyle().Foreground(theme).Render(label)
		}
		b.WriteString(prefix + label + f[1])
		if i < len(fields)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// helpText returns the footer hints for the active screen.
func helpText(s app.Screen, downloading bool) string {
	if downloading {
		return " Please wait... "
	}
	switch s {
	case app.ScreenResume:
		return " [Enter/Y] Resume | [Esc/N] Discard "
	case app.ScreenActivity:
		return " [J/K] Move | [A] Add | [E] Rename | [D] Delete | [S] Settings | [Q] Quit "
	case app.ScreenAddActivity, app.ScreenRenameActivity:
		return " [Enter] Save | [Esc] Cancel "
	case app.ScreenTimer:
		return " [Space] Pause | [+/-] Vol | [M] Mute | [H/Left] Stop & Menu "
	case app.ScreenSettings:
		return " [J/K] Select | [H/L] Change | [Esc/Q] Back "
	case app.ScreenHistory:
		return " [J/K] Scroll | [A] Add | [E] Edit | [D] Delete | [Esc/Q] Back "
	case app.ScreenSessionForm:
		return " [J/K] Select Field | [H/L] Adjust | [Enter] Save | [Esc] Cancel "
	case app.ScreenStats:
		return " [J/K] Scroll | [Esc/Q/Left] Back "
	default:
		return " [Arrows/HJKL] Navigate | [Esc] Back "
	}
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
