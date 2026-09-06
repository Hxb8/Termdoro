package audio

import (
	"math"
	"testing"
)

func TestLinearToVolume(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{1.0, 0},
		{0.5, -1},
		{0.25, -2},
		{2.0, 0}, // clamped
	}
	for _, c := range cases {
		if got := linearToVolume(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("linearToVolume(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := linearToVolume(0); got >= 0 {
		t.Errorf("linearToVolume(0) = %v, want negative", got)
	}
}

func TestIdlePlayerIsSilentNoOp(t *testing.T) {
	p := New()
	// None of these may panic or block without a device or track.
	p.SetVolume(0.5, false)
	p.SetPaused(true)
	p.Stop()
	if err := p.Play("/nonexistent/track.mp3", false); err == nil {
		t.Error("expected error for missing file")
	}
}
