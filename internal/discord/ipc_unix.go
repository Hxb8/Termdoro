//go:build !windows

package discord

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// socketCandidates lists the Unix socket paths the Discord desktop client
// listens on, in priority order.
func socketCandidates() []string {
	var dirs []string
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		dirs = append(dirs, v)
	}
	dirs = append(dirs, os.TempDir(), "/tmp")
	var out []string
	for _, d := range dirs {
		for i := 0; i < 10; i++ {
			out = append(out, filepath.Join(d, fmt.Sprintf("discord-ipc-%d", i)))
		}
	}
	return out
}

func dialIPC() (net.Conn, error) {
	var lastErr error
	for _, path := range socketCandidates() {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("discord: no ipc socket found")
	}
	return nil, lastErr
}
