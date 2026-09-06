package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Hxb8/Termdoro/internal/app"
)

// testPlayer is a silent audio stub.
type testPlayer struct {
	played string
	paused bool
}

func (p *testPlayer) Play(path string, startPaused bool) error {
	p.played = path
	p.paused = startPaused
	return nil
}
func (p *testPlayer) Stop()                            {}
func (p *testPlayer) SetVolume(linear float64, m bool) {}
func (p *testPlayer) SetPaused(paused bool)            { p.paused = paused }

func newTestModel(t *testing.T) *Model {
	t.Helper()
	a := app.New(t.TempDir(), nil)
	a.Player = &testPlayer{}
	m := NewModel(a, nil)
	m.notify = func(title, body string) {}
	m.width, m.height = 100, 30
	return m
}

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func specialKey(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func press(m *Model, msg tea.KeyMsg) *Model {
	model, _ := m.handleKey(msg)
	return model.(*Model)
}

func TestActivityNavigation(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	n := a.MenuLen()

	m = press(m, runeKey("j"))
	if a.Idx != 1 {
		t.Errorf("idx = %d after j", a.Idx)
	}
	m = press(m, specialKey(tea.KeyUp))
	if a.Idx != 0 {
		t.Errorf("idx = %d after up", a.Idx)
	}
	m = press(m, specialKey(tea.KeyUp)) // clamped at top
	if a.Idx != 0 {
		t.Errorf("idx = %d, want clamp 0", a.Idx)
	}
	for i := 0; i < n+5; i++ {
		m = press(m, runeKey("j"))
	}
	if a.Idx != n-1 {
		t.Errorf("idx = %d, want clamp %d", a.Idx, n-1)
	}
	// Enter on an activity goes to the duration screen.
	a.Idx = 0
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenDuration {
		t.Errorf("screen = %v", a.Screen)
	}
}

func TestActivityMenuShortcuts(t *testing.T) {
	m := newTestModel(t)
	a := m.App()

	a.Idx = len(a.Acts) // "Previous Sessions"
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenHistory {
		t.Errorf("screen = %v, want History", a.Screen)
	}
	a.Screen = app.ScreenActivity
	a.Idx = len(a.Acts) + 1 // "Session Stats"
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenStats {
		t.Errorf("screen = %v, want Stats", a.Screen)
	}

	a.Screen = app.ScreenActivity
	m = press(m, runeKey("s"))
	if a.Screen != app.ScreenSettings {
		t.Errorf("screen = %v, want Settings", a.Screen)
	}
	a.Screen = app.ScreenActivity
	m = press(m, runeKey("a"))
	if a.Screen != app.ScreenAddActivity {
		t.Errorf("screen = %v, want AddActivity", a.Screen)
	}
}

func TestAddRenameDeleteActivity(t *testing.T) {
	m := newTestModel(t)
	a := m.App()

	m = press(m, runeKey("a"))
	for _, r := range "My Hobby 🎯" {
		m = press(m, runeKey(string(r)))
	}
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenActivity || a.Acts[len(a.Acts)-1] != "My Hobby 🎯" {
		t.Errorf("after add: screen=%v acts=%v", a.Screen, a.Acts)
	}
	// Empty names are rejected.
	before := len(a.Acts)
	a.Screen = app.ScreenAddActivity
	a.Input = "   "
	m = press(m, specialKey(tea.KeyEnter))
	if len(a.Acts) != before {
		t.Errorf("empty activity added: %v", a.Acts)
	}

	// Rename the custom activity.
	a.Idx = len(a.Acts) - 1
	m = press(m, runeKey("e"))
	if a.Screen != app.ScreenRenameActivity {
		t.Fatalf("screen = %v", a.Screen)
	}
	a.Input = "Renamed 🎯"
	m = press(m, specialKey(tea.KeyEnter))
	if a.Acts[len(a.Acts)-1] != "Renamed 🎯" {
		t.Errorf("acts = %v", a.Acts)
	}
	// Built-in activities cannot be renamed or deleted.
	a.Idx = 0
	m = press(m, runeKey("e"))
	if a.Screen != app.ScreenActivity {
		t.Errorf("built-in rename entered: %v", a.Screen)
	}
	m = press(m, runeKey("d"))
	if len(a.Acts) != before {
		t.Errorf("built-in deleted: %v", a.Acts)
	}
	// Delete the custom one.
	a.Idx = len(a.Acts) - 1
	m = press(m, runeKey("d"))
	if len(a.Acts) != before-1 {
		t.Errorf("acts = %v", a.Acts)
	}
}

func TestDurationSessionsBGMTimerFlow(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	a.Screen = app.ScreenDuration

	m = press(m, runeKey("k"))
	if a.Mins != 26 {
		t.Errorf("mins = %d", a.Mins)
	}
	a.Mins = 1
	m = press(m, runeKey("j")) // clamped at 1
	if a.Mins != 1 {
		t.Errorf("mins = %d, want 1", a.Mins)
	}
	m = press(m, runeKey("l"))
	if a.Screen != app.ScreenSessions {
		t.Errorf("screen = %v", a.Screen)
	}
	m = press(m, runeKey("k"))
	if a.Total != 5 {
		t.Errorf("total = %d", a.Total)
	}
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenBGM {
		t.Errorf("screen = %v", a.Screen)
	}
	// Back navigation.
	m = press(m, specialKey(tea.KeyEsc))
	if a.Screen != app.ScreenSessions {
		t.Errorf("screen = %v", a.Screen)
	}
	m = press(m, specialKey(tea.KeyEnter))
	// Start the timer from BGM.
	a.Mins = 25
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenTimer || a.Rem != 25*60 || a.Current != 1 || a.Paused || !a.Work {
		t.Errorf("timer start: %+v", a)
	}

	// Pause via space, adjust volume, abandon via h.
	m = press(m, specialKey(tea.KeySpace))
	if !a.Paused {
		t.Error("space should pause")
	}
	vol := a.Volume
	m = press(m, runeKey("+"))
	if a.Volume <= vol {
		t.Errorf("volume not raised: %v", a.Volume)
	}
	m = press(m, runeKey("h"))
	if a.Screen != app.ScreenActivity {
		t.Errorf("screen = %v", a.Screen)
	}
	if rows := a.LoadHistoryRaw(); len(rows) != 1 || !strings.HasSuffix(rows[0], ",No") {
		t.Errorf("abandoned session not logged: %v", rows)
	}
}

func TestSettingsToggle(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	a.Screen = app.ScreenSettings

	m = press(m, runeKey("l")) // notifications off
	if a.NotificationsEnabled {
		t.Error("notifications still on")
	}
	m = press(m, runeKey("j"))
	m = press(m, runeKey("l")) // theme forward
	if a.Theme != app.ThemeMagenta {
		t.Errorf("theme = %v", a.Theme)
	}
	m = press(m, runeKey("h")) // theme back
	if a.Theme != app.ThemeCyan {
		t.Errorf("theme = %v", a.Theme)
	}
	m = press(m, runeKey("j"))
	m = press(m, runeKey("h")) // break down
	if a.BreakMins != 4 {
		t.Errorf("break = %d", a.BreakMins)
	}
	m = press(m, specialKey(tea.KeyEsc))
	if a.Screen != app.ScreenActivity {
		t.Errorf("screen = %v", a.Screen)
	}
}

func TestHistoryFormFlow(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	a.Screen = app.ScreenHistory

	// Add a manual session through the form. The date field starts pre-filled
	// with today, so clear it first like a user would.
	m = press(m, runeKey("a"))
	if a.Screen != app.ScreenSessionForm || a.FormEditIdx != nil {
		t.Fatalf("screen = %v", a.Screen)
	}
	for i := 0; i < 8; i++ {
		m = press(m, specialKey(tea.KeyBackspace))
	}
	for _, r := range "20260301" {
		m = press(m, runeKey(string(r)))
	}
	m = press(m, specialKey(tea.KeyDown)) // activity field
	m = press(m, runeKey("l"))
	if a.FormActIdx != 1 {
		t.Errorf("form activity = %d", a.FormActIdx)
	}
	m = press(m, specialKey(tea.KeyEnter))
	if a.Screen != app.ScreenHistory {
		t.Errorf("screen = %v", a.Screen)
	}
	if rows := a.LoadHistoryRaw(); len(rows) != 1 || !strings.HasPrefix(rows[0], "2026-03-01") {
		t.Errorf("rows: %v", rows)
	}
	// Edit it back to another date.
	m = press(m, runeKey("e"))
	if a.FormEditIdx == nil {
		t.Fatal("expected edit mode")
	}
	a.FormDate = "20260401"
	m = press(m, specialKey(tea.KeyEnter))
	if rows := a.LoadHistoryRaw(); !strings.HasPrefix(rows[0], "2026-04-01") {
		t.Errorf("rows after edit: %v", rows)
	}
	// Delete it.
	m = press(m, runeKey("d"))
	if rows := a.LoadHistoryRaw(); len(rows) != 0 {
		t.Errorf("rows after delete: %v", rows)
	}
}

func TestResumeFlow(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	a.Mins = 25
	a.Screen = app.ScreenTimer
	a.Paused = false
	a.Rem = 100
	a.SaveTimerState()

	a2 := app.New(a.DataDir, nil)
	a2.Player = &testPlayer{}
	m2 := NewModel(a2, nil)
	if a2.Screen != app.ScreenResume {
		t.Fatalf("screen = %v", a2.Screen)
	}
	m2 = press(m2, runeKey("y"))
	if a2.Screen != app.ScreenTimer || !a2.Paused || a2.Rem != 100 {
		t.Errorf("resumed: %+v", a2)
	}
	// Discard path.
	a2.Screen = app.ScreenResume
	m2 = press(m2, runeKey("n"))
	if a2.Screen != app.ScreenActivity {
		t.Errorf("screen = %v", a2.Screen)
	}
}

func TestDownloadOverlay(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	m.downloader = func(url, pattern string) error { return nil }
	a.Screen = app.ScreenBGMImport
	a.Input = "https://example.com/watch?v=x"

	model, cmd := m.handleKey(specialKey(tea.KeyEnter))
	m = model.(*Model)
	if !m.downloading {
		t.Fatal("expected downloading state")
	}
	// Keys are swallowed while downloading.
	m = press(m, runeKey("q"))
	if a.Screen != app.ScreenBGMImport {
		t.Errorf("screen changed during download: %v", a.Screen)
	}
	msg := cmd()
	done, ok := msg.(downloadDoneMsg)
	if !ok || done.message != "✅ Success! Press ENTER" {
		t.Errorf("download result: %#v", msg)
	}
	updated, _ := m.Update(done)
	m = updated.(*Model)
	if !m.downloadDone {
		t.Error("expected downloadDone")
	}
	m = press(m, specialKey(tea.KeyEnter))
	if m.downloading || a.Screen != app.ScreenBGM {
		t.Errorf("after ack: downloading=%v screen=%v", m.downloading, a.Screen)
	}
}

func TestDownloadCmdErrors(t *testing.T) {
	ok := downloadCmd("u", "p", func(string, string) error { return nil })()
	if msg := ok.(downloadDoneMsg); msg.message != "✅ Success! Press ENTER" {
		t.Errorf("ok: %q", msg.message)
	}
	missing := downloadCmd("u", "p", func(string, string) error { return errors.New("missing") })()
	if msg := missing.(downloadDoneMsg); msg.message != "❌ yt-dlp missing" {
		t.Errorf("missing: %q", msg.message)
	}
	failed := downloadCmd("u", "p", func(string, string) error { return errors.New("failed") })()
	if msg := failed.(downloadDoneMsg); msg.message != "❌ Failed" {
		t.Errorf("failed: %q", msg.message)
	}
}

func TestTickAndPresence(t *testing.T) {
	m := newTestModel(t)
	a := m.App()
	a.Screen = app.ScreenTimer
	a.Paused = false
	a.Rem = 50
	updated, cmd := m.Update(tickMsg{})
	m = updated.(*Model)
	if a.Rem != 49 {
		t.Errorf("rem = %d", a.Rem)
	}
	if cmd == nil {
		t.Error("tick should re-arm")
	}
}

func TestViewRendersEveryScreen(t *testing.T) {
	screens := []app.Screen{
		app.ScreenResume, app.ScreenActivity, app.ScreenDuration, app.ScreenSessions,
		app.ScreenBGM, app.ScreenBGMImport, app.ScreenSettings, app.ScreenTimer,
		app.ScreenHistory, app.ScreenAddActivity, app.ScreenSessionForm,
		app.ScreenRenameActivity, app.ScreenStats,
	}
	for _, s := range screens {
		m := newTestModel(t)
		m.App().Screen = s
		out := m.View()
		if !strings.Contains(out, "POMODORO TUI") {
			t.Errorf("screen %d: missing header", s)
		}
		if out == "" {
			t.Errorf("screen %d: empty view", s)
		}
	}
	m := newTestModel(t)
	m.downloading = true
	m.downloadMsg = "📥 Downloading to system storage..."
	if out := m.View(); !strings.Contains(out, "Downloading") {
		t.Error("download overlay missing")
	}
}
