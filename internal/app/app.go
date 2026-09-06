// Package app holds the core application state and business logic:
// settings persistence, crash-recovery timer state, session history CSV,
// per-activity statistics, the countdown state machine and audio state.
//
// The UI layer (internal/ui) drives an App value; all externally observable
// file formats and state transitions mirror the original Rust implementation.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Hxb8/Termdoro/internal/store"
)

// DefaultActs are the built-in activities; user-added ones are appended.
var DefaultActs = []string{"Studying 📚", "Coding 💻", "Deep Work 🧠", "Reading 📖"}

// RainSeedName is the file name of the bundled rain sound inside the bgm dir.
const RainSeedName = "Rain_Background.mp3"

// NumFormFields is the number of fields on the add/edit session form.
const NumFormFields = 7

// Theme is the UI accent color.
type Theme int

const (
	ThemeCyan Theme = iota
	ThemeMagenta
	ThemeGreen
	ThemeYellow
	ThemeRed
)

// String renders the theme name exactly as stored in config.ini.
func (t Theme) String() string {
	switch t {
	case ThemeMagenta:
		return "Magenta"
	case ThemeGreen:
		return "Green"
	case ThemeYellow:
		return "Yellow"
	case ThemeRed:
		return "Red"
	default:
		return "Cyan"
	}
}

// ParseTheme parses a config.ini theme value, defaulting to cyan.
func ParseTheme(s string) Theme {
	switch s {
	case "Magenta":
		return ThemeMagenta
	case "Green":
		return ThemeGreen
	case "Yellow":
		return ThemeYellow
	case "Red":
		return ThemeRed
	default:
		return ThemeCyan
	}
}

// Next cycles the theme forward (right arrow behavior).
func (t Theme) Next() Theme {
	switch t {
	case ThemeCyan:
		return ThemeMagenta
	case ThemeMagenta:
		return ThemeGreen
	case ThemeGreen:
		return ThemeYellow
	case ThemeYellow:
		return ThemeRed
	default:
		return ThemeCyan
	}
}

// Prev cycles the theme backward (left arrow behavior).
func (t Theme) Prev() Theme {
	switch t {
	case ThemeCyan:
		return ThemeRed
	case ThemeMagenta:
		return ThemeCyan
	case ThemeGreen:
		return ThemeMagenta
	case ThemeYellow:
		return ThemeGreen
	default:
		return ThemeYellow
	}
}

// Screen identifies the currently visible UI screen.
type Screen int

const (
	ScreenResume Screen = iota
	ScreenActivity
	ScreenDuration
	ScreenSessions
	ScreenBGM
	ScreenBGMImport
	ScreenSettings
	ScreenTimer
	ScreenHistory
	ScreenAddActivity
	ScreenSessionForm
	ScreenRenameActivity
	ScreenStats
)

// ActivityStats aggregates session history per activity.
type ActivityStats struct {
	Name          string
	TotalEntries  uint32
	TotalSessions uint32
	TotalMinutes  uint32
	Completed     uint32
	Abandoned     uint32
}

// AudioPlayer abstracts background-music playback. The production
// implementation lives in internal/audio; tests use a fake or nil.
type AudioPlayer interface {
	Play(path string, startPaused bool) error
	Stop()
	SetVolume(linear float64, muted bool)
	SetPaused(paused bool)
}

// App is the full mutable application state.
type App struct {
	Screen    Screen
	Acts      []string
	Idx       int
	Mins      uint32
	BreakMins uint32
	Total     uint32
	Current   uint32
	Rem       uint32 // remaining seconds in the current phase
	Work      bool
	Paused    bool

	Input string

	BGMList []string
	BGMIdx  int

	NotificationsEnabled bool
	Theme                Theme
	SettingsCursor       int
	Volume               float64 // 0.0 - 1.0
	Muted                bool

	DataDir      string
	SessionStart time.Time

	HistoryScroll int

	// Session form fields.
	FormActIdx    int
	FormMins      uint32
	FormBreakMins uint32
	FormDone      uint32
	FormPlanned   uint32
	FormCompleted bool
	FormDate      string // YYYYMMDD digits
	FormCursor    int
	FormEditIdx   *int

	RenameIdx int

	StatsScroll int

	Player AudioPlayer
}

// New creates an App using dataDir for storage, seeding the bundled rain
// sound when sound is non-empty. It mirrors App::new in the Rust version.
func New(dataDir string, sound []byte) *App {
	_ = os.MkdirAll(filepath.Join(dataDir, "bgm"), 0o755)

	if len(sound) > 0 {
		rainPath := filepath.Join(dataDir, "bgm", RainSeedName)
		if _, err := os.Stat(rainPath); os.IsNotExist(err) {
			_ = os.WriteFile(rainPath, sound, 0o644)
		}
	}

	_, hasSavedState := os.Stat(store.StatePath(dataDir))

	screen := ScreenActivity
	if hasSavedState == nil {
		screen = ScreenResume
	}

	a := &App{
		Screen:               screen,
		Acts:                 append([]string{}, DefaultActs...),
		Mins:                 25,
		BreakMins:            5,
		Total:                4,
		Current:              1,
		Rem:                  25 * 60,
		Work:                 true,
		Paused:               true,
		BGMList:              []string{"None"},
		NotificationsEnabled: true,
		Theme:                ThemeCyan,
		Volume:               0.5,
		DataDir:              dataDir,
		FormMins:             25,
		FormBreakMins:        5,
		FormDone:             4,
		FormPlanned:          4,
		FormCompleted:        true,
		FormDate:             todayDateDigits(),
	}
	a.LoadConfig()
	a.RefreshBGM()
	return a
}

// MenuLen is the number of selectable rows on the activity screen:
// activities plus "Previous Sessions" and "Session Stats".
func (a *App) MenuLen() int { return len(a.Acts) + 2 }

// ---------------------------------------------------------------------------
// Config (config.ini)
// ---------------------------------------------------------------------------

// LoadConfig reads config.ini, ignoring malformed lines and unknown keys.
func (a *App) LoadConfig() {
	content, err := os.ReadFile(store.ConfigPath(a.DataDir))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch key {
		case "break_mins":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.BreakMins = maxUint32(uint32(v), 1)
			}
		case "notifications":
			a.NotificationsEnabled = val == "true"
		case "theme":
			a.Theme = ParseTheme(val)
		case "volume":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.Volume = clampFloat(float64(v)/100.0, 0, 1)
			}
		case "activity":
			if val != "" && !contains(a.Acts, val) {
				a.Acts = append(a.Acts, val)
			}
		}
	}
}

// SaveConfig persists settings and custom activities to config.ini.
func (a *App) SaveConfig() {
	var b strings.Builder
	fmt.Fprintf(&b, "break_mins=%d\n", a.BreakMins)
	fmt.Fprintf(&b, "notifications=%v\n", a.NotificationsEnabled)
	fmt.Fprintf(&b, "theme=%s\n", a.Theme)
	fmt.Fprintf(&b, "volume=%d\n", uint32(a.Volume*100))
	for _, act := range a.Acts[len(DefaultActs):] {
		fmt.Fprintf(&b, "activity=%s\n", act)
	}
	_ = os.WriteFile(store.ConfigPath(a.DataDir), []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Crash-recovery timer state (timer_state.ini)
// ---------------------------------------------------------------------------

// SaveTimerState persists the in-progress timer every tick.
func (a *App) SaveTimerState() {
	var b strings.Builder
	fmt.Fprintf(&b, "idx=%d\n", a.Idx)
	fmt.Fprintf(&b, "mins=%d\n", a.Mins)
	fmt.Fprintf(&b, "break_mins=%d\n", a.BreakMins)
	fmt.Fprintf(&b, "total=%d\n", a.Total)
	fmt.Fprintf(&b, "current=%d\n", a.Current)
	fmt.Fprintf(&b, "rem=%d\n", a.Rem)
	fmt.Fprintf(&b, "work=%v\n", a.Work)
	fmt.Fprintf(&b, "bgm_idx=%d\n", a.BGMIdx)
	_ = os.WriteFile(store.StatePath(a.DataDir), []byte(b.String()), 0o644)
}

// LoadTimerState restores a previously saved timer, returning false when no
// state file exists.
func (a *App) LoadTimerState() bool {
	content, err := os.ReadFile(store.StatePath(a.DataDir))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch key {
		case "idx":
			if v, err := strconv.Atoi(val); err == nil && v >= 0 && v < len(a.Acts) {
				a.Idx = v
			}
		case "mins":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.Mins = maxUint32(uint32(v), 1)
			}
		case "break_mins":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.BreakMins = maxUint32(uint32(v), 1)
			}
		case "total":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.Total = maxUint32(uint32(v), 1)
			}
		case "current":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.Current = maxUint32(uint32(v), 1)
			}
		case "rem":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil {
				a.Rem = uint32(v)
			}
		case "work":
			a.Work = val == "true"
		case "bgm_idx":
			if v, err := strconv.Atoi(val); err == nil && v >= 0 {
				a.BGMIdx = v
			}
		}
	}
	return true
}

// ClearTimerState deletes the crash-recovery file after a normal exit.
func (a *App) ClearTimerState() {
	_ = os.Remove(store.StatePath(a.DataDir))
}

// ResumeInfo summarizes the saved timer for the resume prompt.
type ResumeInfo struct {
	Found    bool
	ActName  string
	Phase    string
	Current  uint32
	Total    uint32
	RemMins  uint32
	RemSecs  uint32
	HasTimer bool
}

// SavedTimerSummary parses timer_state.ini for display on the resume screen.
func (a *App) SavedTimerSummary() ResumeInfo {
	content, err := os.ReadFile(store.StatePath(a.DataDir))
	if err != nil {
		return ResumeInfo{}
	}
	var info ResumeInfo
	var idx int
	var work = true
	hasIdx := false
	for _, line := range strings.Split(string(content), "\n") {
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch key {
		case "current":
			info.Current = parseU32(val)
		case "total":
			info.Total = parseU32(val)
		case "rem":
			rem := parseU32(val)
			info.RemMins, info.RemSecs = rem/60, rem%60
		case "idx":
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				idx, hasIdx = v, true
			}
		case "work":
			work = strings.TrimSpace(val) == "true"
		}
	}
	info.Found = true
	info.HasTimer = true
	if hasIdx && idx >= 0 && idx < len(a.Acts) {
		info.ActName = a.Acts[idx]
	} else {
		info.ActName = "Unknown"
	}
	if work {
		info.Phase = "Focus"
	} else {
		info.Phase = "Break"
	}
	return info
}

// ---------------------------------------------------------------------------
// Session history (sessions.csv)
// ---------------------------------------------------------------------------

const sessionsHeader = "date,activity,duration_mins,break_mins,sessions_planned,sessions_done,completed"

func (a *App) appendSessionLine(line string) {
	logPath := store.SessionsPath(a.DataDir)
	writeHeader := !fileExists(logPath)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if writeHeader {
		fmt.Fprintln(f, sessionsHeader)
	}
	fmt.Fprintln(f, line)
	a.sortSessionsCSV()
}

// LogSession records a finished (completed=true) or abandoned timer run.
func (a *App) LogSession(completed bool) {
	activity := stripCommas(a.Acts[a.Idx])
	var done uint32
	if completed {
		done = a.Total
	} else if a.Current > 0 {
		done = a.Current - 1
	}
	flag := "No"
	if completed {
		flag = "Yes"
	}
	a.appendSessionLine(fmt.Sprintf("%s,%s,%d,%d,%d,%d,%s",
		unixToDate(uint64(time.Now().Unix())), activity, a.Mins, a.BreakMins, a.Total, done, flag))
}

// LogManualSession records the form contents as a new history entry.
func (a *App) LogManualSession() {
	activity := "Unknown"
	if a.FormActIdx >= 0 && a.FormActIdx < len(a.Acts) {
		activity = stripCommas(a.Acts[a.FormActIdx])
	}
	flag := "No"
	if a.FormCompleted {
		flag = "Yes"
	}
	a.appendSessionLine(fmt.Sprintf("%s,%s,%d,%d,%d,%d,%s",
		formatDateCSV(a.FormDate), activity, a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone, flag))
}

// UpdateSessionEntry rewrites the entry shown at displayIdx (most-recent-first)
// with the current form contents.
func (a *App) UpdateSessionEntry(displayIdx int) {
	logPath := store.SessionsPath(a.DataDir)
	content, err := os.ReadFile(logPath)
	if err != nil {
		return
	}
	allLines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	dataCount := len(allLines) - 1
	if dataCount <= 0 || displayIdx < 0 || displayIdx >= dataCount {
		return
	}
	fileLineIdx := dataCount - displayIdx
	activity := "Unknown"
	if a.FormActIdx >= 0 && a.FormActIdx < len(a.Acts) {
		activity = stripCommas(a.Acts[a.FormActIdx])
	}
	flag := "No"
	if a.FormCompleted {
		flag = "Yes"
	}
	allLines[fileLineIdx] = fmt.Sprintf("%s,%s,%d,%d,%d,%d,%s",
		formatDateCSV(a.FormDate), activity, a.FormMins, a.FormBreakMins, a.FormPlanned, a.FormDone, flag)
	_ = os.WriteFile(logPath, []byte(strings.Join(allLines, "\n")+"\n"), 0o644)
	a.sortSessionsCSV()
}

// DeleteHistoryEntry removes the entry shown at displayIdx (most-recent-first)
// and clamps the scroll position.
func (a *App) DeleteHistoryEntry(displayIdx int) {
	logPath := store.SessionsPath(a.DataDir)
	if content, err := os.ReadFile(logPath); err == nil {
		allLines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
		dataCount := len(allLines) - 1
		if dataCount > 0 && displayIdx >= 0 && displayIdx < dataCount {
			fileLineIdx := dataCount - displayIdx
			kept := make([]string, 0, len(allLines)-1)
			for i, line := range allLines {
				if i != fileLineIdx {
					kept = append(kept, line)
				}
			}
			_ = os.WriteFile(logPath, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
		}
	}
	if remaining := len(a.LoadHistoryDisplay()); remaining == 0 {
		a.HistoryScroll = 0
	} else if a.HistoryScroll >= remaining {
		a.HistoryScroll = remaining - 1
	}
}

// RenameActivityInCSV rewrites every history row using oldName to newName.
func (a *App) RenameActivityInCSV(oldName, newName string) {
	logPath := store.SessionsPath(a.DataDir)
	content, err := os.ReadFile(logPath)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 7 && parts[1] == oldName {
			parts[1] = newName
			lines[i] = strings.Join(parts, ",")
		}
	}
	_ = os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// LoadStats aggregates the session history per activity.
func (a *App) LoadStats() []ActivityStats {
	content, err := os.ReadFile(store.SessionsPath(a.DataDir))
	if err != nil {
		return nil
	}
	var stats []ActivityStats
	for i, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
		if i == 0 {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			continue
		}
		name := parts[1]
		mins := parseU32(parts[2])
		done := parseU32(parts[5])
		completed := strings.TrimSpace(parts[6]) == "Yes"
		totalMins := mins * done
		found := false
		for j := range stats {
			if stats[j].Name == name {
				stats[j].TotalEntries++
				stats[j].TotalSessions += done
				stats[j].TotalMinutes += totalMins
				if completed {
					stats[j].Completed++
				} else {
					stats[j].Abandoned++
				}
				found = true
				break
			}
		}
		if !found {
			s := ActivityStats{Name: name, TotalEntries: 1, TotalSessions: done, TotalMinutes: totalMins}
			if completed {
				s.Completed = 1
			} else {
				s.Abandoned = 1
			}
			stats = append(stats, s)
		}
	}
	return stats
}

// LoadHistoryRaw returns CSV data rows oldest-first (header excluded).
func (a *App) LoadHistoryRaw() []string {
	content, err := os.ReadFile(store.SessionsPath(a.DataDir))
	if err != nil {
		return nil
	}
	var lines []string
	for i, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
		if i == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// LoadHistoryDisplay returns human-readable rows most-recent-first.
func (a *App) LoadHistoryDisplay() []string {
	raw := a.LoadHistoryRaw()
	display := make([]string, 0, len(raw))
	for _, line := range raw {
		parts := strings.Split(line, ",")
		switch {
		case len(parts) >= 7:
			display = append(display, fmt.Sprintf("%s | %s | %smin work / %smin break | %s/%s sessions | %s",
				parts[0], parts[1], parts[2], parts[3], parts[5], parts[4], parts[6]))
		case len(parts) >= 6:
			display = append(display, fmt.Sprintf("%s | %s | %smin | %s/%s sessions | %s",
				parts[0], parts[1], parts[2], parts[4], parts[3], parts[5]))
		}
	}
	for i, j := 0, len(display)-1; i < j; i, j = i+1, j-1 {
		display[i], display[j] = display[j], display[i]
	}
	return display
}

// sortSessionsCSV sorts data rows ascending by date (stable for equal dates).
func (a *App) sortSessionsCSV() {
	logPath := store.SessionsPath(a.DataDir)
	content, err := os.ReadFile(logPath)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(lines) <= 2 {
		return
	}
	header, data := lines[0], lines[1:]
	kept := data[:0]
	for _, l := range data {
		if l != "" {
			kept = append(kept, l)
		}
	}
	// Insertion sort keeps the implementation dependency-free and stable.
	for i := 1; i < len(kept); i++ {
		for j := i; j > 0 && dateField(kept[j]) < dateField(kept[j-1]); j-- {
			kept[j], kept[j-1] = kept[j-1], kept[j]
		}
	}
	out := append([]string{header}, kept...)
	_ = os.WriteFile(logPath, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// ---------------------------------------------------------------------------
// Session form
// ---------------------------------------------------------------------------

// InitFormForAdd resets the form for a new manual entry.
func (a *App) InitFormForAdd() {
	a.FormActIdx = 0
	a.FormMins = a.Mins
	a.FormBreakMins = a.BreakMins
	a.FormDone = 4
	a.FormPlanned = 4
	a.FormCompleted = true
	a.FormDate = todayDateDigits()
	a.FormCursor = 0
	a.FormEditIdx = nil
}

// InitFormForEdit loads the entry shown at displayIdx into the form.
func (a *App) InitFormForEdit(displayIdx int) {
	raw := a.LoadHistoryRaw()
	if displayIdx < 0 || displayIdx >= len(raw) {
		return
	}
	line := raw[len(raw)-1-displayIdx]
	parts := strings.Split(line, ",")
	if len(parts) >= 7 {
		a.FormDate = strings.ReplaceAll(parts[0], "-", "")
		actName := parts[1]
		a.FormActIdx = 0
		for i, act := range a.Acts {
			if stripCommas(act) == actName {
				a.FormActIdx = i
				break
			}
		}
		a.FormMins = orDefault(parseU32(parts[2]), 25)
		a.FormBreakMins = orDefault(parseU32(parts[3]), 5)
		a.FormPlanned = orDefault(parseU32(parts[4]), 4)
		a.FormDone = parseU32(parts[5])
		a.FormCompleted = strings.TrimSpace(parts[6]) == "Yes"
	}
	a.FormCursor = 0
	idx := displayIdx
	a.FormEditIdx = &idx
}

// ClampHistoryScroll keeps the history cursor inside the list bounds.
func (a *App) ClampHistoryScroll() {
	if n := len(a.LoadHistoryDisplay()); n == 0 {
		a.HistoryScroll = 0
	} else if a.HistoryScroll > n-1 {
		a.HistoryScroll = n - 1
	} else if a.HistoryScroll < 0 {
		a.HistoryScroll = 0
	}
}

// ClampStatsScroll keeps the stats cursor inside the list bounds.
func (a *App) ClampStatsScroll() {
	if n := len(a.LoadStats()); n == 0 {
		a.StatsScroll = 0
	} else if a.StatsScroll > n-1 {
		a.StatsScroll = n - 1
	} else if a.StatsScroll < 0 {
		a.StatsScroll = 0
	}
}

// ---------------------------------------------------------------------------
// BGM
// ---------------------------------------------------------------------------

// RefreshBGM rebuilds the track list: "None" plus every .mp3 in the bgm dir.
func (a *App) RefreshBGM() {
	list := []string{"None"}
	entries, err := os.ReadDir(store.BGMDir(a.DataDir))
	if err != nil {
		a.BGMList = list
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mp3") {
			list = append(list, e.Name())
		}
	}
	a.BGMList = list
}

// PlayBGM starts the selected track; index 0 ("None") stops playback.
func (a *App) PlayBGM() {
	a.StopBGM()
	if a.Player == nil || a.BGMIdx <= 0 || a.BGMIdx >= len(a.BGMList) {
		return
	}
	path := filepath.Join(store.BGMDir(a.DataDir), a.BGMList[a.BGMIdx])
	if err := a.Player.Play(path, a.Paused); err == nil {
		a.Player.SetVolume(a.Volume, a.Muted)
	}
}

// StopBGM halts playback and releases the audio resources.
func (a *App) StopBGM() {
	if a.Player != nil {
		a.Player.Stop()
	}
}

// AdjustVolume shifts the volume by delta, clamped to [0, 1].
func (a *App) AdjustVolume(delta float64) {
	a.Volume = clampFloat(a.Volume+delta, 0, 1)
	if !a.Muted && a.Player != nil {
		a.Player.SetVolume(a.Volume, false)
	}
}

// ToggleMute flips the mute flag and applies it to the player.
func (a *App) ToggleMute() {
	a.Muted = !a.Muted
	if a.Player != nil {
		a.Player.SetVolume(a.Volume, a.Muted)
	}
}

// TogglePause flips the paused flag and pauses/resumes the player.
func (a *App) TogglePause() {
	a.Paused = !a.Paused
	if a.Player != nil {
		a.Player.SetPaused(a.Paused)
	}
}

// ---------------------------------------------------------------------------
// Countdown state machine
// ---------------------------------------------------------------------------

// OnTick advances the timer by one second. It is a no-op unless the timer
// screen is active, unpaused and has time remaining. Phase transitions fire
// the notify callback when non-nil.
func (a *App) OnTick(notify func(title, body string)) {
	if a.Screen != ScreenTimer || a.Paused || a.Rem == 0 {
		return
	}
	a.Rem--
	if a.Rem == 0 {
		var title, body string
		switch {
		case a.Work && a.Current >= a.Total:
			title, body = "Done! 🎉", "All sessions finished!"
			a.LogSession(true)
			a.ClearTimerState()
			a.Screen = ScreenActivity
			a.StopBGM()
		case a.Work:
			a.Work = false
			a.Rem = a.BreakMins * 60
			title, body = "Break! ☕", "Time to rest."
		default:
			a.Work = true
			a.Current++
			a.Rem = a.Mins * 60
			title, body = "Work! 🔥", "Focus time."
		}
		if a.NotificationsEnabled && notify != nil {
			notify(title, body)
		}
		a.Paused = true
		if a.Player != nil {
			a.Player.SetPaused(true)
		}
	}
	if a.Screen == ScreenTimer {
		a.SaveTimerState()
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func splitKV(line string) (key, val string, ok bool) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

func parseU32(s string) uint32 {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

func orDefault(v, def uint32) uint32 {
	if v == 0 {
		return def
	}
	return v
}

func maxUint32(v, min uint32) uint32 {
	if v < min {
		return min
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func stripCommas(s string) string { return strings.ReplaceAll(s, ",", "") }

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dateField(line string) string {
	if i := strings.Index(line, ","); i >= 0 {
		return line[:i]
	}
	return line
}
