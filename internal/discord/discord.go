// Package discord implements the minimal Discord Rich Presence IPC protocol
// needed by the application, using only the standard library (plus a small
// named-pipe helper on Windows).
//
// Wire format (both transports): uint32 LE opcode, uint32 LE length,
// followed by the JSON payload. Opcode 0 is the handshake, opcode 1 carries
// commands such as SET_ACTIVITY.
package discord

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// AppID is the Discord application ID used for Rich Presence.
const AppID = "1516166471623901294"

const (
	opHandshake = 0
	opFrame     = 1
)

// Activity mirrors the Discord activity object sent via SET_ACTIVITY.
type Activity struct {
	State      string      `json:"state,omitempty"`
	Details    string      `json:"details,omitempty"`
	Assets     *Assets     `json:"assets,omitempty"`
	Timestamps *Timestamps `json:"timestamps,omitempty"`
}

// Assets carries the large application icon key.
type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
}

// Timestamps carries the countdown end time (Unix seconds).
type Timestamps struct {
	End int64 `json:"end,omitempty"`
}

type setActivityArgs struct {
	PID      int      `json:"pid"`
	Activity Activity `json:"activity"`
}

type setActivityCmd struct {
	Cmd   string          `json:"cmd"`
	Args  setActivityArgs `json:"args"`
	Nonce string          `json:"nonce"`
}

type handshake struct {
	V        int    `json:"v"`
	ClientID string `json:"client_id"`
}

// BuildPayload builds the SET_ACTIVITY command payload for the given values.
func BuildPayload(pid int, state, details string, end *int64, nonce string) []byte {
	act := Activity{State: state, Details: details, Assets: &Assets{LargeImage: "app_icon"}}
	if end != nil {
		act.Timestamps = &Timestamps{End: *end}
	}
	cmd := setActivityCmd{Cmd: "SET_ACTIVITY", Args: setActivityArgs{PID: pid, Activity: act}, Nonce: nonce}
	raw, _ := json.Marshal(cmd)
	return raw
}

// EncodeFrame wraps a payload with the opcode/length header.
func EncodeFrame(opcode uint32, payload []byte) []byte {
	frame := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], opcode)
	binary.LittleEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame
}

// Client is a best-effort Discord IPC connection. All methods are safe for
// concurrent use; failures are reported so the caller can retry later.
type Client struct {
	appID string
	mu    sync.Mutex
	conn  net.Conn
}

// New returns a disconnected client for the given application ID.
func New(appID string) *Client { return &Client{appID: appID} }

// Connect dials the Discord client (trying ipc-0..9) and performs the handshake.
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

// Close drops the connection, if any.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

// Reconnect drops the current connection and dials again, ignoring errors.
func (c *Client) Reconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	_ = c.connectLocked()
}

// SetActivity publishes the given presence state. The end Unix timestamp is
// optional (nil means no countdown). On failure the connection is dropped so
// the next call reconnects.
func (c *Client) SetActivity(state, details string, end *int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}
	nonce := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	frame := EncodeFrame(opFrame, BuildPayload(os.Getpid(), state, details, end, nonce))
	if err := c.writeFrameLocked(frame); err != nil {
		c.closeLocked()
		return err
	}
	if _, err := c.readFrameLocked(2 * time.Second); err != nil {
		c.closeLocked()
		return err
	}
	return nil
}

func (c *Client) connectLocked() error {
	c.closeLocked()
	conn, err := dialIPC()
	if err != nil {
		return err
	}
	hs, _ := json.Marshal(handshake{V: 1, ClientID: c.appID})
	if err := writeFrame(conn, EncodeFrame(opHandshake, hs), 2*time.Second); err != nil {
		_ = conn.Close()
		return err
	}
	if _, err := readFrame(conn, 2*time.Second); err != nil {
		_ = conn.Close()
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) writeFrameLocked(frame []byte) error {
	return writeFrame(c.conn, frame, 2*time.Second)
}

func (c *Client) readFrameLocked(timeout time.Duration) ([]byte, error) {
	return readFrame(c.conn, timeout)
}

func writeFrame(conn net.Conn, frame []byte, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	for len(frame) > 0 {
		n, err := conn.Write(frame)
		if err != nil {
			return err
		}
		frame = frame[n:]
	}
	return nil
}

func readFrame(conn net.Conn, timeout time.Duration) ([]byte, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	header := make([]byte, 8)
	if err := readFull(conn, header); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > 1<<20 {
		return nil, fmt.Errorf("discord: frame too large (%d bytes)", length)
	}
	payload := make([]byte, length)
	if err := readFull(conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func readFull(conn net.Conn, buf []byte) error {
	for len(buf) > 0 {
		n, err := conn.Read(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}
