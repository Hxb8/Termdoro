package discord

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildPayload(t *testing.T) {
	end := int64(1788384000)
	raw := BuildPayload(1234, "🔥 Focusing: Coding 💻", "Session 1 of 4", &end, "nonce-1")
	var cmd struct {
		Cmd   string `json:"cmd"`
		Nonce string `json:"nonce"`
		Args  struct {
			PID      int `json:"pid"`
			Activity struct {
				State   string `json:"state"`
				Details string `json:"details"`
				Assets  struct {
					LargeImage string `json:"large_image"`
				} `json:"assets"`
				Timestamps struct {
					End int64 `json:"end"`
				} `json:"timestamps"`
			} `json:"activity"`
		} `json:"args"`
	}
	if err := json.Unmarshal(raw, &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Cmd != "SET_ACTIVITY" || cmd.Nonce != "nonce-1" || cmd.Args.PID != 1234 {
		t.Errorf("envelope: %+v", cmd)
	}
	if cmd.Args.Activity.State != "🔥 Focusing: Coding 💻" || cmd.Args.Activity.Details != "Session 1 of 4" {
		t.Errorf("activity: %+v", cmd.Args.Activity)
	}
	if cmd.Args.Activity.Assets.LargeImage != "app_icon" {
		t.Errorf("assets: %+v", cmd.Args.Activity.Assets)
	}
	if cmd.Args.Activity.Timestamps.End != end {
		t.Errorf("end: %+v", cmd.Args.Activity.Timestamps)
	}
}

func TestBuildPayloadWithoutEnd(t *testing.T) {
	raw := BuildPayload(1, "Configuring...", "Main Menu", nil, "n")
	if strings.Contains(string(raw), "timestamps") {
		t.Errorf("timestamps should be omitted without end: %s", raw)
	}
}

func TestEncodeFrame(t *testing.T) {
	payload := []byte(`{"v":1}`)
	frame := EncodeFrame(0, payload)
	if len(frame) != 8+len(payload) {
		t.Fatalf("frame len = %d", len(frame))
	}
	if op := binary.LittleEndian.Uint32(frame[0:4]); op != 0 {
		t.Errorf("opcode = %d", op)
	}
	if ln := binary.LittleEndian.Uint32(frame[4:8]); ln != uint32(len(payload)) {
		t.Errorf("length = %d", ln)
	}
}

// fakeServer speaks enough of the IPC protocol for one handshake plus one
// SET_ACTIVITY round-trip, then reports the received payloads.
func fakeServer(t *testing.T, sock string) (gotHandshake, gotActivity chan []byte) {
	t.Helper()
	gotHandshake = make(chan []byte, 1)
	gotActivity = make(chan []byte, 1)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		for i := 0; i < 2; i++ {
			header := make([]byte, 8)
			if err := readFull(conn, header); err != nil {
				return
			}
			n := binary.LittleEndian.Uint32(header[4:8])
			payload := make([]byte, n)
			if err := readFull(conn, payload); err != nil {
				return
			}
			if i == 0 {
				gotHandshake <- payload
				_, _ = conn.Write(EncodeFrame(1, []byte(`{"cmd":"DISPATCH","evt":"READY"}`)))
			} else {
				gotActivity <- payload
				_, _ = conn.Write(EncodeFrame(1, []byte(`{"cmd":"SET_ACTIVITY","evt":null}`)))
			}
		}
	}()
	return gotHandshake, gotActivity
}

func TestClientRoundTrip(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	sock := filepath.Join(runtimeDir, "discord-ipc-0")
	gotHandshake, gotActivity := fakeServer(t, sock)

	c := New(AppID)
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	select {
	case hs := <-gotHandshake:
		var v struct {
			V        int    `json:"v"`
			ClientID string `json:"client_id"`
		}
		if err := json.Unmarshal(hs, &v); err != nil {
			t.Fatal(err)
		}
		if v.V != 1 || v.ClientID != AppID {
			t.Errorf("handshake: %+v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no handshake received")
	}

	end := int64(1788384000)
	if err := c.SetActivity("state", "details", &end); err != nil {
		t.Fatalf("set activity: %v", err)
	}
	select {
	case raw := <-gotActivity:
		var cmd map[string]any
		if err := json.Unmarshal(raw, &cmd); err != nil {
			t.Fatal(err)
		}
		if cmd["cmd"] != "SET_ACTIVITY" {
			t.Errorf("cmd = %v", cmd["cmd"])
		}
		args := cmd["args"].(map[string]any)
		act := args["activity"].(map[string]any)
		if act["state"] != "state" || act["details"] != "details" {
			t.Errorf("activity = %v", act)
		}
		ts := act["timestamps"].(map[string]any)
		if ts["end"] != float64(end) {
			t.Errorf("end = %v", ts["end"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no activity received")
	}
}

func TestClientNoServer(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", empty)
	t.Setenv("TMPDIR", empty)
	c := New(AppID)
	if err := c.Connect(); err == nil {
		t.Error("expected connection error without a server")
		_ = c.Close()
	}
	// SetActivity without a server fails but stays usable afterwards.
	if err := c.SetActivity("s", "d", nil); err == nil {
		t.Error("expected error without a server")
	}
	c.Reconnect() // must not panic
	_ = c.Close()
}

func TestSocketCandidates(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	cands := socketCandidates()
	if len(cands) == 0 || cands[0] != fmt.Sprintf("/run/user/1000/discord-ipc-0") {
		t.Errorf("candidates[0] = %v", cands[:1])
	}
}
