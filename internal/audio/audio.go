// Package audio provides fail-soft background-music playback.
//
// It mirrors the rodio-based playback in the Rust version: a track loops
// indefinitely, supports linear 0-1 volume, mute and pause. When no audio
// device is available (headless CI, missing drivers), every method degrades
// to a silent no-op instead of crashing the application.
package audio

import (
	"math"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
)

// Player plays a single looping track. It is safe for concurrent use.
type Player struct {
	mu       sync.Mutex
	streamer beep.StreamSeekCloser
	ctrl     *beep.Ctrl
	vol      *effects.Volume
	rate     beep.SampleRate
	ready    bool // speaker initialized
	noAudio  bool // permanent fallback after init failure
	paused   bool
}

// New returns an idle player. The speaker is initialized lazily on Play.
func New() *Player { return &Player{} }

// Play stops any current track and loops path indefinitely. When startPaused
// is set the track begins paused, mirroring the Rust play_bgm behavior.
func (p *Player) Play(path string, startPaused bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	streamer, format, err := mp3.Decode(f)
	if err != nil {
		_ = f.Close()
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	p.paused = startPaused

	if p.noAudio {
		_ = streamer.Close()
		return nil
	}
	if !p.ready {
		if !p.initSpeaker(format.SampleRate) {
			p.noAudio = true
			_ = streamer.Close()
			return nil
		}
		p.ready = true
		p.rate = format.SampleRate
	}

	looped, err := beep.Loop2(streamer)
	if err != nil {
		_ = streamer.Close()
		return err
	}
	var s beep.Streamer = looped
	if format.SampleRate != p.rate {
		s = beep.Resample(4, format.SampleRate, p.rate, s)
	}
	p.streamer = streamer
	p.ctrl = &beep.Ctrl{Streamer: s, Paused: startPaused}
	// Volume is applied through vol when known; default to full volume.
	p.vol = &effects.Volume{Streamer: p.ctrl, Base: 2, Volume: 0}
	speaker.Play(p.vol)
	return nil
}

// Stop halts playback and releases the current track.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

// SetVolume applies a linear 0-1 volume level and mute flag.
func (p *Player) SetVolume(linear float64, muted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.noAudio || !p.ready || p.vol == nil {
		return
	}
	speaker.Lock()
	p.vol.Volume = linearToVolume(linear)
	p.vol.Silent = muted || linear <= 0
	speaker.Unlock()
}

// SetPaused pauses or resumes playback.
func (p *Player) SetPaused(paused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = paused
	if p.noAudio || !p.ready || p.ctrl == nil {
		return
	}
	speaker.Lock()
	p.ctrl.Paused = paused
	speaker.Unlock()
}

func (p *Player) stopLocked() {
	if p.ready && !p.noAudio {
		speaker.Lock()
		if p.ctrl != nil {
			p.ctrl.Streamer = nil
		}
		speaker.Unlock()
	}
	if p.streamer != nil {
		_ = p.streamer.Close()
		p.streamer = nil
	}
	p.ctrl = nil
	p.vol = nil
}

// initSpeaker initializes the shared speaker. It reports false when no audio
// device is usable; panics from the underlying driver are swallowed so the
// application keeps running without sound.
func (p *Player) initSpeaker(rate beep.SampleRate) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if err := speaker.Init(rate, rate.N(time.Second/10)); err != nil {
		return false
	}
	return true
}

// linearToVolume maps a linear 0-1 level to beep's exponential volume units
// (base 2, so 0.5 is one unit quieter than full volume).
func linearToVolume(linear float64) float64 {
	if linear <= 0 {
		return -10
	}
	if linear > 1 {
		linear = 1
	}
	return math.Log2(linear)
}
