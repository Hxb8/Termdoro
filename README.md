# 🍅 Termdoro

Focus like a pro! A high-performance Terminal User Interface (TUI) Pomodoro timer written in Go. Sync your sessions with Discord, listen to your favorite YouTube BGM, and track your productivity over time.

> Special thanks to [SillyIngenuity](https://github.com/SillyIngenuity), whose fork added session tracking, custom activities, persistent settings, productivity stats and Windows fixes — all of which are preserved in this version.

## Features

🎮 **Discord Rich Presence** — Live countdown timer and pause status on your Discord profile.

🎶 **YouTube BGM Importer** — Download and play background music directly from YouTube (requires yt-dlp and ffmpeg).

📋 **Session History** — Completed and abandoned sessions are logged locally to a CSV file and browsable from the main menu, most recent first.

✏️ **Add & Edit Sessions** — Manually log sessions you finished away from your PC, or edit existing entries (date, activity, duration, break time, session counts, completion status).

📊 **Session Stats** — Per-activity totals: sessions, time spent, entries, average sessions per entry, completed vs abandoned.

🎯 **Custom Activities** — Add your own activities beyond the defaults; they persist across launches. Custom activities can be renamed (history updates automatically) or deleted.

⏸️ **Break Duration Setting** — Customize your break length from Settings.

💾 **Persistent Settings** — Theme, break duration, notifications, volume and custom activities save to a local config file.

🛟 **Crash-Safe Timer** — Timer state saves every second; after an unexpected close you are prompted to resume.

⌨️ **Vim-Style Navigation** — Full HJKL and Arrow key support.

🎨 **Custom Themes** — Cyan, Magenta, Green, Yellow, and Red.

🔔 **System Notifications** — Get notified when a session or break ends.

⚡ **Lightweight & Fast** — Single static binary, negligible idle CPU usage.

## Installation

### Go install (recommended)

Requires Go 1.24 or newer:

```
go install github.com/Hxb8/Termdoro/cmd/termdoro@latest
```

This installs the `termdoro` binary into your `$GOPATH/bin` (or `$HOME/go/bin`). Make sure it is on your `PATH`, then just run:

```
termdoro
```

### Prebuilt binaries

Go to the [Releases](https://github.com/Hxb8/Termdoro/releases) page and download the archive for your platform (`termdoro-linux.tar.gz`, `termdoro-macos.tar.gz`, `termdoro-windows.zip`). Extract and run `termdoro`.

Or use the installer script:

```
curl -fsSL https://raw.githubusercontent.com/Hxb8/Termdoro/HEAD/install.sh | bash
```

This installs the binary as `termdoro` in `/usr/local/bin`.

### Build from Source

Requires Go 1.24+ (and ALSA headers, e.g. `libasound2-dev`, on Linux for audio):

```
git clone https://github.com/Hxb8/Termdoro
cd Termdoro
go build -o termdoro ./cmd/termdoro
```

## Keybinds

### Main Menu
| Key | Action |
|-----|--------|
| J/K or Arrows | Navigate |
| Enter/L/Right | Select |
| A | Add custom activity |
| E | Rename custom activity |
| D | Delete custom activity |
| S | Settings |
| Q | Quit |

### Timer
| Key | Action |
|-----|--------|
| Space | Pause/Resume |
| +/- | Volume up/down |
| M | Mute/Unmute |
| H/Left/Esc | Stop & return to menu |

### Session History
| Key | Action |
|-----|--------|
| J/K | Scroll |
| A | Add manual session |
| E | Edit selected session |
| D | Delete selected session |
| Esc/Q/Left | Back |

### Session Form (Add/Edit)
| Key | Action |
|-----|--------|
| J/K | Select field |
| H/L | Adjust value |
| 0-9 | Type date (on Date field) |
| Backspace | Delete date digit |
| Enter | Save |
| Esc | Cancel |

### Session Stats
| Key | Action |
|-----|--------|
| J/K | Scroll |
| Esc/Q/Left | Back |

### Settings
| Key | Action |
|-----|--------|
| J/K | Select option |
| H/L | Change value |
| Esc/Q | Back |

## Data Storage

All data is stored locally at:

- **Windows:** `%LOCALAPPDATA%\pomodoro-tui\`
- **Linux:** `~/.local/share/pomodoro-tui/` (or `$XDG_DATA_HOME/pomodoro-tui/`)
- **macOS:** `~/Library/Application Support/pomodoro-tui/`

Files:

- `config.ini` — Settings and custom activities
- `sessions.csv` — Session history (used by stats and history screens, auto-sorted by date)
- `timer_state.ini` — Crash recovery state (auto-deleted on normal exit)
- `bgm/` — Downloaded background music

## Project Structure

```
cmd/termdoro/      CLI entry point (wires everything together)
assets/            Embedded rain-sound.mp3 seeded into bgm/ on first run
internal/app/      Core state: settings, timer state machine, sessions CSV,
                   stats, history, forms, volume/mute/pause
internal/store/    Platform data-directory resolution
internal/discord/  Minimal Discord Rich Presence IPC client (stdlib only)
internal/audio/    Looping MP3 playback with volume/mute/pause (fail-soft)
internal/ui/       Bubble Tea model (key handling) + Lipgloss views
```

## Development

```
go build ./...        # build everything
go vet ./...          # static analysis
gofmt -l cmd internal assets  # formatting check
go test ./...         # unit tests
go test -race ./...   # tests with the race detector
```

Optional tools used at runtime:

- `yt-dlp` (+ `ffmpeg`) for the YouTube BGM importer
- A running Discord desktop client for Rich Presence
- `notify-send` (Linux) for desktop notifications

## Privacy

This application is open source. It only communicates locally with your Discord client via IPC. No personal data is collected or sent to any server. Network access is limited to the optional YouTube BGM downloader which uses yt-dlp.

## Credits

Original project by [Islam/Hexbyte](https://github.com/hexbyte16). Big thanks to [SillyIngenuity](https://github.com/SillyIngenuity) for the enhanced fork this version builds on.

Made with ❤️ and 🐹
