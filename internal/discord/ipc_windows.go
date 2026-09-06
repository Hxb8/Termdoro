//go:build windows

package discord

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialIPC() (net.Conn, error) {
	var lastErr error
	for i := 0; i < 10; i++ {
		conn, err := winio.DialPipe(fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i), nil)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("discord: no ipc pipe found")
	}
	return nil, lastErr
}
