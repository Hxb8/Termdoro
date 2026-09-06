package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hxb8/Termdoro/internal/store"
)

// fakePlayer records audio calls without touching any device.
type fakePlayer struct {
	played  string
	stopped int
	volume  float64
	muted   bool
	paused  bool
}

func (f *fakePlayer) Play(path string, startPaused bool) error {
	f.played = path
	f.paused = startPaused
	return nil
}
func (f *fakePlayer) Stop()                            { f.stopped++ }
func (f *fakePlayer) SetVolume(linear float64, m bool) { f.volume, f.muted = linear, m }
func (f *fakePlayer) SetPaused(p bool)                 { f.paused = p }

func newTestApp(t *testing.T) *App {
	t.Helper()
	a := New(t.TempDir(), nil)
	a.Player = &fakePlayer{}
	return a
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestNewDefaults(t *testing.T) {
	a := newTestApp(t)
	if a.Screen != ScreenActivity {
		t.Errorf("fresh app screen = %v, want Activity", a.Screen)
	}
	if len(a.Acts) != len(DefaultActs) {
		t.Errorf("acts = %v, want defaults", a.Acts)
	}
	if a.Mins != 25 || a.BreakMins != 5 || a.Total != 4 || a.Current != 1 {
		t.Errorf("bad timer defaults: %+v", a)
	}
	if a.Rem != 25*60 || !a.Work || !a.Paused {
		t.Errorf("bad countdown defaults: %+v", a)
	}
	if !a.NotificationsEnabled || a.Theme != ThemeCyan || a.Volume != 0.5 {
		t.Errorf("bad settings defaults: %+v", a)
	}
	if a.MenuLen() != len(DefaultActs)+2 {
		t.Errorf("menu len = %d", a.MenuLen())
	}
	// The bgm dir is created and the list starts with "None".
	if len(a.BGMList) != 1 || a.BGMList[0] != "None" {
		t.Errorf("bgm list = %v", a.BGMList)
	}
}

func TestNewSeedsRainSound(t *testing.T) {
	dir := t.TempDir()
	New(dir, []byte("fake-mp3-bytes"))
	raw, err := os.ReadFile(filepath.Join(dir, "bgm", RainSeedName))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "fake-mp3-bytes" {
		t.Errorf("seeded sound = %q", raw)
	}
	// A second launch must not overwrite an existing file.
	New(dir, []byte("other"))
	if got := readFile(t, filepath.Join(dir, "bgm", RainSeedName)); got != "fake-mp3-bytes" {
		t.Errorf("seed overwrote existing file: %q", got)
	}
}

func TestNewResumeScreenWhenStateExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(store.StatePath(dir), []byte("rem=60\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := New(dir, nil).Screen; got != ScreenResume {
		t.Errorf("screen = %v, want Resume", got)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	a := newTestApp(t)
	a.BreakMins = 10
	a.NotificationsEnabled = false
	a.Theme = ThemeGreen
	a.Volume = 0.75
	a.Acts = append(a.Acts, "Actuary Exam 📝")
	a.SaveConfig()

	b := New(a.DataDir, nil)
	if b.BreakMins != 10 || b.NotificationsEnabled || b.Theme != ThemeGreen {
		t.Errorf("config not restored: %+v", b)
	}
	if b.Volume != 0.75 {
		t.Errorf("volume = %v, want 0.75", b.Volume)
	}
	if len(b.Acts) != len(DefaultActs)+1 || b.Acts[len(b.Acts)-1] != "Actuary Exam 📝" {
		t.Errorf("acts = %v", b.Acts)
	}

	content := readFile(t, store.ConfigPath(a.DataDir))
	for _, want := range []string{"break_mins=10", "notifications=false", "theme=Green", "volume=75", "activity=Actuary Exam 📝"} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %q:\n%s", want, content)
		}
	}
}

func TestLoadConfigRobustness(t *testing.T) {
	a := newTestApp(t)
	raw := "garbage line\nbreak_mins=0\nbreak_mins=abc\nvolume=250\nvolume=xyz\n" +
		"notifications=maybe\ntheme=Invisible\nactivity=\nactivity=Coding 💻\nactivity=New 🎯\nunknown=1\n"
	if err := os.WriteFile(store.ConfigPath(a.DataDir), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	a.LoadConfig()
	if a.BreakMins != 1 {
		t.Errorf("break_mins = %d, want clamped 1", a.BreakMins)
	}
	if a.Volume != 1 {
		t.Errorf("volume = %v, want clamped 1", a.Volume)
	}
	if a.NotificationsEnabled {
		t.Error("notifications should be false for non-true value")
	}
	if a.Theme != ThemeCyan {
		t.Errorf("theme = %v, want Cyan default", a.Theme)
	}
	if len(a.Acts) != len(DefaultActs)+1 || a.Acts[len(a.Acts)-1] != "New 🎯" {
		t.Errorf("acts = %v, want one appended custom activity", a.Acts)
	}
}

func TestTimerStateRoundTrip(t *testing.T) {
	a := newTestApp(t)
	a.Idx = 1
	a.Mins = 50
	a.BreakMins = 15
	a.Total = 6
	a.Current = 3
	a.Rem = 1234
	a.Work = false
	a.BGMIdx = 2
	a.SaveTimerState()

	b := New(a.DataDir, nil)
	if !b.LoadTimerState() {
		t.Fatal("LoadTimerState returned false")
	}
	if b.Idx != 1 || b.Mins != 50 || b.BreakMins != 15 || b.Total != 6 ||
		b.Current != 3 || b.Rem != 1234 || b.Work || b.BGMIdx != 2 {
		t.Errorf("state not restored: %+v", b)
	}
	b.ClearTimerState()
	if _, err := os.Stat(store.StatePath(a.DataDir)); !os.IsNotExist(err) {
		t.Error("state file not deleted")
	}
	if New(a.DataDir, nil).LoadTimerState() {
		t.Error("LoadTimerState should fail without a file")
	}
}

func TestLoadTimerStateClampsAndBounds(t *testing.T) {
	a := newTestApp(t)
	raw := "idx=99\nmins=0\nbreak_mins=0\ntotal=0\ncurrent=0\nrem=5\nwork=false\nbgm_idx=3\n"
	if err := os.WriteFile(store.StatePath(a.DataDir), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if !a.LoadTimerState() {
		t.Fatal("expected true")
	}
	if a.Idx != 0 {
		t.Errorf("idx = %d, want 0 (out of range ignored)", a.Idx)
	}
	if a.Mins != 1 || a.BreakMins != 1 || a.Total != 1 || a.Current != 1 {
		t.Errorf("counts not clamped to 1: %+v", a)
	}
	if a.Rem != 5 || a.Work {
		t.Errorf("rem/work wrong: %+v", a)
	}
	// bgm_idx is intentionally unclamped (playback guards it).
	if a.BGMIdx != 3 {
		t.Errorf("bgm_idx = %d, want 3", a.BGMIdx)
	}
}

func TestLogSessionCompletedAndAbandoned(t *testing.T) {
	a := newTestApp(t)
	a.Mins = 25
	a.BreakMins = 5
	a.Total = 4
	a.Current = 2
	a.LogSession(true)
	a.LogSession(false)

	lines := strings.Split(strings.TrimSpace(readFile(t, store.SessionsPath(a.DataDir))), "\n")
	if len(lines) != 3 {
		t.Fatalf("csv lines = %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if lines[0] != "date,activity,duration_mins,break_mins,sessions_planned,sessions_done,completed" {
		t.Errorf("bad header: %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], ",Studying 📚,25,5,4,4,Yes") {
		t.Errorf("completed line: %q", lines[1])
	}
	// Abandoned on session 2 of 4 records 1 done session.
	if !strings.HasSuffix(lines[2], ",Studying 📚,25,5,4,1,No") {
		t.Errorf("abandoned line: %q", lines[2])
	}
	// Activity commas are stripped.
	a.Acts = append(a.Acts, "Weird,Name")
	a.Idx = len(a.Acts) - 1
	a.LogSession(true)
	lines = strings.Split(strings.TrimSpace(readFile(t, store.SessionsPath(a.DataDir))), "\n")
	if strings.Contains(lines[3], "Weird,Name") {
		t.Errorf("commas not stripped: %q", lines[3])
	}
}

func TestSortSessionsCSV(t *testing.T) {
	a := newTestApp(t)
	for _, d := range []string{"2026-09-06", "2026-01-01", "2025-12-31"} {
		a.FormDate = strings.ReplaceAll(d, "-", "")
		a.FormActIdx = 0
		a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone = 25, 5, 4, 4
		a.FormCompleted = true
		a.LogManualSession()
	}
	raw := a.LoadHistoryRaw()
	if len(raw) != 3 {
		t.Fatalf("rows = %d", len(raw))
	}
	for i, want := range []string{"2025-12-31", "2026-01-01", "2026-09-06"} {
		if !strings.HasPrefix(raw[i], want) {
			t.Errorf("row %d = %q, want date prefix %q", i, raw[i], want)
		}
	}
	// Display order is most-recent-first.
	display := a.LoadHistoryDisplay()
	if !strings.HasPrefix(display[0], "2026-09-06") || !strings.HasPrefix(display[2], "2025-12-31") {
		t.Errorf("display order wrong: %v", display)
	}
	want := "2026-09-06 | Studying 📚 | 25min work / 5min break | 4/4 sessions | Yes"
	if display[0] != want {
		t.Errorf("display[0] = %q, want %q", display[0], want)
	}
}

func TestUpdateAndDeleteHistoryEntry(t *testing.T) {
	a := newTestApp(t)
	for _, d := range []string{"20260101", "20260201", "20260301"} {
		a.FormDate = d
		a.FormActIdx = 0
		a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone = 25, 5, 4, 4
		a.FormCompleted = true
		a.LogManualSession()
	}
	// Display index 0 is the most recent (March). Rewrite it as February work.
	a.FormDate = "20260215"
	a.FormMins = 50
	a.UpdateSessionEntry(0)
	raw := a.LoadHistoryRaw()
	if !strings.HasPrefix(raw[2], "2026-02-15") || !strings.Contains(raw[2], ",50,") {
		t.Errorf("updated rows: %v", raw)
	}
	// Out-of-range edits are ignored.
	a.UpdateSessionEntry(9)
	if got := len(a.LoadHistoryRaw()); got != 3 {
		t.Errorf("rows = %d after bad edit", got)
	}

	// Delete the oldest entry via its display index (2).
	a.HistoryScroll = 2
	a.DeleteHistoryEntry(2)
	raw = a.LoadHistoryRaw()
	if len(raw) != 2 || !strings.HasPrefix(raw[0], "2026-02-01") || !strings.HasPrefix(raw[1], "2026-02-15") {
		t.Errorf("rows after delete: %v", raw)
	}
	if a.HistoryScroll != 1 {
		t.Errorf("scroll = %d, want clamped 1", a.HistoryScroll)
	}
	// Bad indexes never touch the file.
	a.DeleteHistoryEntry(7)
	if got := len(a.LoadHistoryRaw()); got != 2 {
		t.Errorf("rows = %d after bad delete", got)
	}
}

func TestRenameActivityInCSV(t *testing.T) {
	a := newTestApp(t)
	a.Acts = append(a.Acts, "Old Name")
	a.FormActIdx = len(a.Acts) - 1
	a.FormDate = "20260301"
	a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone = 25, 5, 4, 4
	a.FormCompleted = true
	a.LogManualSession()
	a.RenameActivityInCSV("Old Name", "New Name")
	raw := a.LoadHistoryRaw()
	if !strings.Contains(raw[0], ",New Name,") {
		t.Errorf("row not renamed: %q", raw[0])
	}
}

func TestLoadStats(t *testing.T) {
	a := newTestApp(t)
	seed := []struct {
		act       string
		mins      uint32
		done      uint32
		completed bool
	}{
		{"Coding 💻", 25, 4, true},
		{"Coding 💻", 25, 2, false},
		{"Reading 📖", 30, 1, true},
	}
	for _, s := range seed {
		a.FormActIdx = indexOf(a.Acts, s.act)
		a.FormDate = "20260301"
		a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone = s.mins, 5, 4, s.done
		a.FormCompleted = s.completed
		a.LogManualSession()
	}
	stats := a.LoadStats()
	if len(stats) != 2 {
		t.Fatalf("stats = %+v", stats)
	}
	byName := map[string]ActivityStats{}
	for _, s := range stats {
		byName[s.Name] = s
	}
	coding := byName["Coding 💻"]
	if coding.TotalEntries != 2 || coding.TotalSessions != 6 || coding.TotalMinutes != 150 {
		t.Errorf("coding stats: %+v", coding)
	}
	if coding.Completed != 1 || coding.Abandoned != 1 {
		t.Errorf("coding done/quit: %+v", coding)
	}
	reading := byName["Reading 📖"]
	if reading.TotalSessions != 1 || reading.TotalMinutes != 30 || reading.Completed != 1 {
		t.Errorf("reading stats: %+v", reading)
	}
	if got := newTestApp(t).LoadStats(); len(got) != 0 {
		t.Errorf("empty history should yield no stats: %+v", got)
	}
}

func TestFormInit(t *testing.T) {
	a := newTestApp(t)
	a.Mins, a.BreakMins = 50, 15
	a.InitFormForAdd()
	if a.FormMins != 50 || a.FormBreakMins != 15 || !a.FormCompleted || a.FormEditIdx != nil {
		t.Errorf("add form: %+v", a)
	}
	if len(a.FormDate) != 8 {
		t.Errorf("add form date = %q", a.FormDate)
	}

	a.Acts = append(a.Acts, "Custom 🎯")
	a.FormActIdx = len(a.Acts) - 1
	a.FormDate = "20260301"
	a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone = 30, 10, 5, 3
	a.FormCompleted = false
	a.LogManualSession()
	a.InitFormForEdit(0)
	if a.FormDate != "20260301" || a.FormMins != 30 || a.FormBreakMins != 10 ||
		a.FormPlanned != 5 || a.FormDone != 3 || a.FormCompleted {
		t.Errorf("edit form: %+v", a)
	}
	if a.FormActIdx != len(a.Acts)-1 {
		t.Errorf("edit form activity idx = %d", a.FormActIdx)
	}
	if a.FormEditIdx == nil || *a.FormEditIdx != 0 {
		t.Errorf("edit idx = %v", a.FormEditIdx)
	}
	// Unknown activities fall back to index 0.
	if err := os.WriteFile(store.SessionsPath(a.DataDir),
		[]byte("date,activity,duration_mins,break_mins,sessions_planned,sessions_done,completed\n2026-01-01,Gone,25,5,4,4,Yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.InitFormForEdit(0)
	if a.FormActIdx != 0 {
		t.Errorf("missing activity idx = %d, want 0", a.FormActIdx)
	}
	a.InitFormForEdit(5) // out of range: no-op
}

func TestOnTick(t *testing.T) {
	t.Run("counts down and saves state", func(t *testing.T) {
		a := newTestApp(t)
		a.Screen = ScreenTimer
		a.Paused = false
		a.Rem = 10
		var notified int
		a.OnTick(func(title, body string) { notified++ })
		if a.Rem != 9 || notified != 0 {
			t.Errorf("rem = %d, notified = %d", a.Rem, notified)
		}
		if _, err := os.Stat(store.StatePath(a.DataDir)); err != nil {
			t.Error("timer state should be saved each tick")
		}
	})

	t.Run("idle when paused or off-screen", func(t *testing.T) {
		a := newTestApp(t)
		a.Screen = ScreenTimer
		a.Paused = true
		a.Rem = 10
		a.OnTick(nil)
		if a.Rem != 10 {
			t.Errorf("paused tick decremented to %d", a.Rem)
		}
		a.Paused = false
		a.Screen = ScreenActivity
		a.OnTick(nil)
		if a.Rem != 10 {
			t.Errorf("off-screen tick decremented to %d", a.Rem)
		}
	})

	t.Run("work to break transition", func(t *testing.T) {
		a := newTestApp(t)
		a.Screen = ScreenTimer
		a.Paused = false
		a.Work = true
		a.Current, a.Total = 1, 4
		a.BreakMins = 5
		a.Rem = 1
		var titles []string
		a.OnTick(func(title, body string) { titles = append(titles, title) })
		if a.Work || a.Rem != 5*60 || !a.Paused {
			t.Errorf("after break start: %+v", a)
		}
		if len(titles) != 1 || titles[0] != "Break! ☕" {
			t.Errorf("titles = %v", titles)
		}
	})

	t.Run("break to work transition", func(t *testing.T) {
		a := newTestApp(t)
		a.Screen = ScreenTimer
		a.Paused = false
		a.Work = false
		a.Current, a.Total = 1, 4
		a.Mins = 25
		a.Rem = 1
		var titles []string
		a.OnTick(func(title, body string) { titles = append(titles, title) })
		if !a.Work || a.Current != 2 || a.Rem != 25*60 {
			t.Errorf("after work start: %+v", a)
		}
		if len(titles) != 1 || titles[0] != "Work! 🔥" {
			t.Errorf("titles = %v", titles)
		}
	})

	t.Run("final session completes", func(t *testing.T) {
		a := newTestApp(t)
		a.Screen = ScreenTimer
		a.Paused = false
		a.Work = true
		a.Current, a.Total = 4, 4
		a.Rem = 1
		var titles []string
		a.OnTick(func(title, body string) { titles = append(titles, title) })
		if a.Screen != ScreenActivity {
			t.Errorf("screen = %v, want Activity", a.Screen)
		}
		if _, err := os.Stat(store.StatePath(a.DataDir)); !os.IsNotExist(err) {
			t.Error("state file should be cleared on completion")
		}
		if rows := a.LoadHistoryRaw(); len(rows) != 1 || !strings.HasSuffix(rows[0], ",4,Yes") {
			t.Errorf("history rows: %v", rows)
		}
		if p := a.Player.(*fakePlayer); p.stopped != 1 || !p.paused {
			t.Errorf("player not paused on transition: %+v", p)
		}
		if len(titles) != 1 || titles[0] != "Done! 🎉" {
			t.Errorf("titles = %v", titles)
		}
	})

	t.Run("notifications can be disabled", func(t *testing.T) {
		a := newTestApp(t)
		a.Screen = ScreenTimer
		a.Paused = false
		a.Work = true
		a.Current, a.Total = 1, 4
		a.Rem = 1
		a.NotificationsEnabled = false
		called := false
		a.OnTick(func(title, body string) { called = true })
		if called {
			t.Error("notify called while disabled")
		}
	})
}

func TestVolumeAndPauseControls(t *testing.T) {
	a := newTestApp(t)
	p := a.Player.(*fakePlayer)

	a.AdjustVolume(0.6)
	if a.Volume != 1 {
		t.Errorf("volume = %v, want clamped 1", a.Volume)
	}
	a.AdjustVolume(-2)
	if a.Volume != 0 || p.volume != 0 {
		t.Errorf("volume = %v player = %v", a.Volume, p.volume)
	}
	a.Volume = 0.5
	a.Muted = true
	a.AdjustVolume(0.1) // muted: player untouched
	if p.volume != 0 {
		t.Errorf("muted adjust touched player: %v", p.volume)
	}
	a.ToggleMute()
	if a.Muted || p.muted {
		t.Errorf("mute toggle: app=%v player=%v", a.Muted, p.muted)
	}
	a.TogglePause() // fresh apps start paused, mirroring the Rust default
	if a.Paused || p.paused {
		t.Errorf("pause toggle: app=%v player=%v", a.Paused, p.paused)
	}
}

func TestPlayBGMGuards(t *testing.T) {
	a := newTestApp(t)
	p := a.Player.(*fakePlayer)
	a.PlayBGM() // "None" selected: no-op
	if p.played != "" {
		t.Errorf("played = %q with None selected", p.played)
	}
	a.BGMIdx = 99 // out of range: guarded
	a.PlayBGM()
	if p.played != "" || p.stopped != 2 {
		t.Errorf("guards failed: %+v", p)
	}
	// A real file plays paused-aware with volume applied.
	if err := os.WriteFile(filepath.Join(a.DataDir, "bgm", "x.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.RefreshBGM()
	a.BGMIdx = 1
	a.Paused = true
	a.Volume = 0.3
	a.PlayBGM()
	if !strings.HasSuffix(p.played, "x.mp3") || !p.paused || p.volume != 0.3 {
		t.Errorf("play: %+v", p)
	}
}

func TestThemeCycle(t *testing.T) {
	if ThemeCyan.Next() != ThemeMagenta || ThemeRed.Next() != ThemeCyan {
		t.Error("bad Next cycle")
	}
	if ThemeCyan.Prev() != ThemeRed || ThemeMagenta.Prev() != ThemeCyan {
		t.Error("bad Prev cycle")
	}
	cases := map[string]Theme{"Cyan": ThemeCyan, "Magenta": ThemeMagenta, "Green": ThemeGreen, "Yellow": ThemeYellow, "Red": ThemeRed, "bogus": ThemeCyan}
	for in, want := range cases {
		if got := ParseTheme(in); got != want {
			t.Errorf("ParseTheme(%q) = %v", in, got)
		}
		if Theme(want).String() != in && in != "bogus" {
			t.Errorf("String() = %q, want %q", Theme(want).String(), in)
		}
	}
}

func TestSavedTimerSummary(t *testing.T) {
	a := newTestApp(t)
	if got := a.SavedTimerSummary(); got.Found {
		t.Error("no state file: Found should be false")
	}
	a.Idx, a.Mins, a.BreakMins, a.Total, a.Current, a.Rem, a.Work = 0, 25, 5, 4, 2, 125, false
	a.SaveTimerState()
	s := a.SavedTimerSummary()
	if !s.Found || s.ActName != "Studying 📚" || s.Phase != "Break" || s.Current != 2 || s.Total != 4 || s.RemMins != 2 || s.RemSecs != 5 {
		t.Errorf("summary: %+v", s)
	}
}

func TestClampScrolls(t *testing.T) {
	a := newTestApp(t)
	a.HistoryScroll, a.StatsScroll = 5, 5
	a.ClampHistoryScroll()
	a.ClampStatsScroll()
	if a.HistoryScroll != 0 || a.StatsScroll != 0 {
		t.Errorf("empty clamps: %d %d", a.HistoryScroll, a.StatsScroll)
	}
	a.FormDate = "20260301"
	a.LogManualSession()
	a.HistoryScroll = 9
	a.ClampHistoryScroll()
	if a.HistoryScroll != 0 {
		t.Errorf("history scroll = %d", a.HistoryScroll)
	}
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return 0
}
