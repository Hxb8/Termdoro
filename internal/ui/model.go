// Package ui implements the terminal interface as a Bubble Tea application.
//
// The interaction model mirrors the original Ratatui event loop: one screen
// at a time, vim-style (HJKL/arrows) navigation, a one-second countdown tick
// and Discord presence refresh, and a yt-dlp download overlay that captures
// all input while active.
package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/beeep"

	"github.com/Hxb8/Termdoro/internal/app"
	"github.com/Hxb8/Termdoro/internal/discord"
)

// tickMsg fires once per second to advance the countdown.
type tickMsg time.Time

// downloadDoneMsg reports the outcome of a background yt-dlp download.
type downloadDoneMsg struct {
	message string
}

// Downloader downloads url as mp3 into outputPattern (a yt-dlp -o template).
type Downloader func(url, outputPattern string) error

// DefaultDownloader shells out to yt-dlp exactly like the Rust version.
func DefaultDownloader(url, outputPattern string) error {
	cmd := exec.Command("yt-dlp", "-x", "--audio-format", "mp3",
		"--quiet", "--no-warnings", "-o", outputPattern, url)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return errors.New("failed")
		}
		return errors.New("missing")
	}
	return nil
}

// NotifyFunc shows a desktop notification for phase transitions.
type NotifyFunc func(title, body string)

// DefaultNotify uses cross-platform system notifications.
func DefaultNotify(title, body string) {
	_ = beeep.Notify(title, body, "")
}

// Model is the Bubble Tea model driving the whole application.
type Model struct {
	app        *app.App
	presence   *discord.Client
	notify     NotifyFunc
	downloader Downloader

	width  int
	height int

	downloading  bool
	downloadDone bool
	downloadMsg  string

	quitting bool
}

// NewModel wires the application core to its collaborators.
func NewModel(a *app.App, presence *discord.Client) *Model {
	return &Model{
		app:        a,
		presence:   presence,
		notify:     DefaultNotify,
		downloader: DefaultDownloader,
		width:      80,
		height:     24,
	}
}

// App exposes the underlying application state (used by main and tests).
func (m *Model) App() *app.App { return m.app }

// Init starts the per-second tick.
func (m *Model) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update routes messages to the tick handler, the download handler or the
// active screen's key handler.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil
	case tickMsg:
		m.app.OnTick(func(title, body string) {
			if m.app.NotificationsEnabled && m.notify != nil {
				m.notify(title, body)
			}
		})
		m.updatePresence()
		return m, tickCmd()
	case downloadDoneMsg:
		m.downloadMsg = msg.message
		m.downloadDone = true
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey mirrors the Rust match on (screen, key code). While a download is
// in progress every key is swallowed except Enter once the download finishes.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := m.app
	if m.downloading {
		if m.downloadDone && (msg.Type == tea.KeyEnter) {
			m.downloading = false
			m.downloadDone = false
			a.RefreshBGM()
			a.Screen = app.ScreenBGM
		}
		return m, nil
	}

	key := msg.String()
	switch a.Screen {
	case app.ScreenResume:
		switch key {
		case "enter", "y":
			if a.LoadTimerState() {
				a.Paused = true
				a.Screen = app.ScreenTimer
				a.PlayBGM()
			} else {
				a.ClearTimerState()
				a.Screen = app.ScreenActivity
			}
		case "esc", "n", "q":
			a.ClearTimerState()
			a.Screen = app.ScreenActivity
		}

	case app.ScreenActivity:
		switch key {
		case "up", "k":
			if a.Idx > 0 {
				a.Idx--
			}
		case "down", "j":
			if a.Idx < a.MenuLen()-1 {
				a.Idx++
			}
		case "enter", "l", "right":
			switch {
			case a.Idx == len(a.Acts):
				a.HistoryScroll = 0
				a.Screen = app.ScreenHistory
			case a.Idx == len(a.Acts)+1:
				a.StatsScroll = 0
				a.Screen = app.ScreenStats
			default:
				a.Screen = app.ScreenDuration
			}
		case "a":
			a.Input = ""
			a.Screen = app.ScreenAddActivity
		case "e":
			if a.Idx >= len(app.DefaultActs) && a.Idx < len(a.Acts) {
				a.RenameIdx = a.Idx
				a.Input = a.Acts[a.Idx]
				a.Screen = app.ScreenRenameActivity
			}
		case "d", "delete":
			if a.Idx >= len(app.DefaultActs) && a.Idx < len(a.Acts) {
				a.Acts = append(a.Acts[:a.Idx], a.Acts[a.Idx+1:]...)
				a.SaveConfig()
				if a.Idx >= a.MenuLen() {
					a.Idx = a.MenuLen() - 1
				}
			}
		case "s":
			a.SettingsCursor = 0
			a.Screen = app.ScreenSettings
		case "q":
			m.quitting = true
			return m, tea.Quit
		}

	case app.ScreenAddActivity:
		switch msg.Type {
		case tea.KeyEnter:
			if name := trimSpace(a.Input); name != "" && !containsStr(a.Acts, name) {
				a.Acts = append(a.Acts, name)
				a.SaveConfig()
			}
			a.Input = ""
			a.Screen = app.ScreenActivity
		case tea.KeyBackspace:
			a.Input = popRune(a.Input)
		case tea.KeyEsc:
			a.Input = ""
			a.Screen = app.ScreenActivity
		case tea.KeyRunes:
			a.Input += msg.String()
		}

	case app.ScreenRenameActivity:
		switch msg.Type {
		case tea.KeyEnter:
			if name := trimSpace(a.Input); name != "" && !containsStr(a.Acts, name) {
				oldName := stripCommas(a.Acts[a.RenameIdx])
				cleanNew := stripCommas(name)
				a.Acts[a.RenameIdx] = name
				a.RenameActivityInCSV(oldName, cleanNew)
				a.SaveConfig()
			}
			a.Input = ""
			a.Screen = app.ScreenActivity
		case tea.KeyBackspace:
			a.Input = popRune(a.Input)
		case tea.KeyEsc:
			a.Input = ""
			a.Screen = app.ScreenActivity
		case tea.KeyRunes:
			a.Input += msg.String()
		}

	case app.ScreenDuration:
		switch key {
		case "up", "k":
			a.Mins++
		case "down", "j":
			if a.Mins > 1 {
				a.Mins--
			}
		case "enter", "l", "right":
			a.Screen = app.ScreenSessions
		case "h", "left", "esc":
			a.Screen = app.ScreenActivity
		}

	case app.ScreenSessions:
		switch key {
		case "up", "k":
			a.Total++
		case "down", "j":
			if a.Total > 1 {
				a.Total--
			}
		case "enter", "l", "right":
			a.Screen = app.ScreenBGM
		case "h", "left", "esc":
			a.Screen = app.ScreenDuration
		}

	case app.ScreenBGM:
		switch key {
		case "i":
			a.Screen = app.ScreenBGMImport
			a.Input = ""
		case "up", "k":
			if a.BGMIdx > 0 {
				a.BGMIdx--
			}
		case "down", "j":
			if a.BGMIdx < len(a.BGMList)-1 {
				a.BGMIdx++
			}
		case "enter", "l", "right":
			a.Rem = a.Mins * 60
			a.Current = 1
			a.Screen = app.ScreenTimer
			a.Work = true
			a.Paused = false
			a.SessionStart = time.Now()
			a.SaveTimerState()
			a.PlayBGM()
		case "h", "left", "esc":
			a.Screen = app.ScreenSessions
		}

	case app.ScreenBGMImport:
		switch msg.Type {
		case tea.KeyEnter:
			url, pattern := m.startDownload()
			return m, downloadCmd(url, pattern, m.downloader)
		case tea.KeyBackspace:
			a.Input = popRune(a.Input)
		case tea.KeyEsc:
			a.Screen = app.ScreenBGM
		case tea.KeyRunes:
			a.Input += msg.String()
		}

	case app.ScreenSettings:
		switch key {
		case "up", "k":
			if a.SettingsCursor > 0 {
				a.SettingsCursor--
			}
		case "down", "j":
			if a.SettingsCursor < 2 {
				a.SettingsCursor++
			}
		case "left", "h":
			switch a.SettingsCursor {
			case 0:
				a.NotificationsEnabled = !a.NotificationsEnabled
			case 1:
				a.Theme = a.Theme.Prev()
			case 2:
				if a.BreakMins > 1 {
					a.BreakMins--
				}
			}
			a.SaveConfig()
		case "right", "l":
			switch a.SettingsCursor {
			case 0:
				a.NotificationsEnabled = !a.NotificationsEnabled
			case 1:
				a.Theme = a.Theme.Next()
			case 2:
				a.BreakMins++
			}
			a.SaveConfig()
		case "esc", "q":
			a.Screen = app.ScreenActivity
		}

	case app.ScreenTimer:
		switch key {
		case " ":
			a.TogglePause()
			a.SaveTimerState()
		case "m", "M":
			a.ToggleMute()
		case "+", "=":
			a.AdjustVolume(0.05)
			a.SaveConfig()
		case "-", "_":
			a.AdjustVolume(-0.05)
			a.SaveConfig()
		case "q", "h", "left", "esc":
			a.LogSession(false)
			a.ClearTimerState()
			a.StopBGM()
			a.Screen = app.ScreenActivity
		}

	case app.ScreenHistory:
		switch key {
		case "up", "k":
			if a.HistoryScroll > 0 {
				a.HistoryScroll--
			}
		case "down", "j":
			a.HistoryScroll++
		case "d", "delete":
			if count := len(a.LoadHistoryDisplay()); count > 0 && a.HistoryScroll < count {
				a.DeleteHistoryEntry(a.HistoryScroll)
			}
		case "e":
			if count := len(a.LoadHistoryDisplay()); count > 0 && a.HistoryScroll < count {
				a.InitFormForEdit(a.HistoryScroll)
				a.Screen = app.ScreenSessionForm
			}
		case "a":
			a.InitFormForAdd()
			a.Screen = app.ScreenSessionForm
		case "esc", "q", "left":
			a.Screen = app.ScreenActivity
		}

	case app.ScreenSessionForm:
		switch key {
		case "up", "k":
			if a.FormCursor > 0 {
				a.FormCursor--
			}
		case "down", "j":
			if a.FormCursor < app.NumFormFields-1 {
				a.FormCursor++
			}
		case "right", "l":
			switch a.FormCursor {
			case 1:
				if a.FormActIdx < len(a.Acts)-1 {
					a.FormActIdx++
				}
			case 2:
				a.FormMins++
			case 3:
				a.FormBreakMins++
			case 4:
				a.FormDone++
			case 5:
				a.FormPlanned++
			case 6:
				a.FormCompleted = !a.FormCompleted
			}
		case "left", "h":
			switch a.FormCursor {
			case 1:
				if a.FormActIdx > 0 {
					a.FormActIdx--
				}
			case 2:
				if a.FormMins > 1 {
					a.FormMins--
				}
			case 3:
				if a.FormBreakMins > 1 {
					a.FormBreakMins--
				}
			case 4:
				if a.FormDone > 0 {
					a.FormDone--
				}
			case 5:
				if a.FormPlanned > 1 {
					a.FormPlanned--
				}
			case 6:
				a.FormCompleted = !a.FormCompleted
			}
		case "enter":
			if a.FormEditIdx != nil {
				a.UpdateSessionEntry(*a.FormEditIdx)
			} else {
				a.LogManualSession()
			}
			a.Screen = app.ScreenHistory
		case "esc", "q":
			a.Screen = app.ScreenHistory
		default:
			if msg.Type == tea.KeyRunes && a.FormCursor == 0 && len(a.FormDate) < 8 && isDigitStr(key) {
				a.FormDate += key
			} else if msg.Type == tea.KeyBackspace && a.FormCursor == 0 {
				if len(a.FormDate) > 0 {
					a.FormDate = a.FormDate[:len(a.FormDate)-1]
				}
			}
		}

	case app.ScreenStats:
		switch key {
		case "up", "k":
			if a.StatsScroll > 0 {
				a.StatsScroll--
			}
		case "down", "j":
			a.StatsScroll++
		case "esc", "q", "left":
			a.Screen = app.ScreenActivity
		}
	}
	return m, nil
}

// startDownload captures the URL and flips the downloading flags, mirroring
// the Rust thread that sets is_downloading. It returns the URL and the yt-dlp
// output pattern for the background command.
func (m *Model) startDownload() (url, pattern string) {
	url = trimSpace(m.app.Input)
	pattern = m.app.DataDir + "/bgm/%(title)s.%(ext)s"
	m.app.Input = ""
	m.downloading = true
	m.downloadDone = false
	m.downloadMsg = "📥 Downloading to system storage..."
	return url, pattern
}

// downloadCmd runs the blocking download and maps the outcome to the exact
// status strings used by the Rust version.
func downloadCmd(url, pattern string, downloader Downloader) tea.Cmd {
	return func() tea.Msg {
		var message string
		switch err := downloader(url, pattern); {
		case err == nil:
			message = "✅ Success! Press ENTER"
		case err.Error() == "missing":
			message = "❌ yt-dlp missing"
		default:
			message = "❌ Failed"
		}
		return downloadDoneMsg{message: message}
	}
}

// updatePresence refreshes Discord Rich Presence in the background so a slow
// or missing Discord client never blocks the UI.
func (m *Model) updatePresence() {
	if m.presence == nil {
		return
	}
	a := m.app
	var state, details string
	var end *int64
	if a.Screen == app.ScreenTimer {
		if a.Idx >= 0 && a.Idx < len(a.Acts) {
			if a.Paused {
				state = fmt.Sprintf("⏸️ Paused: %s", a.Acts[a.Idx])
			} else if !a.Work {
				state = "☕ Taking a Break"
			} else {
				state = fmt.Sprintf("🔥 Focusing: %s", a.Acts[a.Idx])
			}
		}
		details = fmt.Sprintf("Session %d of %d", a.Current, a.Total)
		if !a.Paused {
			e := time.Now().Unix() + int64(a.Rem)
			end = &e
		}
	} else {
		state = "Configuring..."
		details = "Main Menu"
	}
	go func() {
		if err := m.presence.SetActivity(state, details, end); err != nil {
			m.presence.Reconnect()
		}
	}()
}

// ---------------------------------------------------------------------------
// small helpers (mirroring the Rust string handling)
// ---------------------------------------------------------------------------

func trimSpace(s string) string { return strings.TrimSpace(s) }

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func stripCommas(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != ',' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// popRune removes the last rune (Rust's String::pop semantics).
func popRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

func isDigitStr(s string) bool {
	return len(s) == 1 && s[0] >= '0' && s[0] <= '9'
}
