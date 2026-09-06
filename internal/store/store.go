// Package store resolves the on-disk locations used by the application.
//
// It mirrors the behavior of the Rust `dirs::data_dir()` crate function:
//   - Windows: %LOCALAPPDATA%\pomodoro-tui\
//   - Linux:   $XDG_DATA_HOME/pomodoro-tui/ or ~/.local/share/pomodoro-tui/
//   - macOS:   ~/Library/Application Support/pomodoro-tui/
package store

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppDirName is the directory name appended to the platform data directory.
const AppDirName = "pomodoro-tui"

// DataDir returns the platform-specific data directory for the application,
// creating it (including the bgm subdirectory) if necessary.
// It falls back to "./pomodoro-tui" when no home directory can be determined,
// mirroring the Rust `unwrap_or_else(|| PathBuf::from("."))` behavior.
func DataDir() string {
	base := userDataBase()
	dir := filepath.Join(base, AppDirName)
	_ = os.MkdirAll(filepath.Join(dir, "bgm"), 0o755)
	return dir
}

// userDataBase mirrors dirs::data_dir() without the trailing app name.
func userDataBase() string {
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v
		}
		if v := os.Getenv("APPDATA"); v != "" {
			return v
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support")
		}
	default: // linux and other unix-likes
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return v
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

// ConfigPath returns the path of config.ini inside dir.
func ConfigPath(dir string) string { return filepath.Join(dir, "config.ini") }

// StatePath returns the path of timer_state.ini inside dir.
func StatePath(dir string) string { return filepath.Join(dir, "timer_state.ini") }

// SessionsPath returns the path of sessions.csv inside dir.
func SessionsPath(dir string) string { return filepath.Join(dir, "sessions.csv") }

// BGMDir returns the background-music directory inside dir.
func BGMDir(dir string) string { return filepath.Join(dir, "bgm") }
