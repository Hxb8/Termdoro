package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirXDG(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	dir := DataDir()
	if want := filepath.Join(base, AppDirName); dir != want {
		t.Errorf("DataDir() = %q, want %q", dir, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "bgm")); err != nil {
		t.Errorf("bgm dir not created: %v", err)
	}
}

func TestDataDirFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := DataDir()
	if want := filepath.Join(home, ".local", "share", AppDirName); dir != want {
		t.Errorf("DataDir() = %q, want %q", dir, want)
	}
}

func TestPaths(t *testing.T) {
	dir := filepath.Join("some", "dir")
	if !strings.HasSuffix(ConfigPath(dir), "config.ini") {
		t.Errorf("config path: %q", ConfigPath(dir))
	}
	if !strings.HasSuffix(StatePath(dir), "timer_state.ini") {
		t.Errorf("state path: %q", StatePath(dir))
	}
	if !strings.HasSuffix(SessionsPath(dir), "sessions.csv") {
		t.Errorf("sessions path: %q", SessionsPath(dir))
	}
	if !strings.HasSuffix(BGMDir(dir), "bgm") {
		t.Errorf("bgm path: %q", BGMDir(dir))
	}
}
