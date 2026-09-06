package app

import (
	"strings"
	"testing"
)

func TestUnixToDate(t *testing.T) {
	cases := []struct {
		secs uint64
		want string
	}{
		{0, "1970-01-01"},
		{86400, "1970-01-02"},
		{1582934400, "2020-02-29"}, // leap day
		{1709164800, "2024-02-29"}, // leap day
		{1719792000, "2024-07-01"},
		{1788384000, "2026-09-02"},
		{4102444800, "2100-01-01"}, // century, not a leap year
	}
	for _, c := range cases {
		if got := unixToDate(c.secs); got != c.want {
			t.Errorf("unixToDate(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestFormatDateDisplay(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"20260906", "2026-09-06"},
		{"2026", "2026-__-__"},
		{"", "____-__-__"},
		{"2026090", "2026-09-0_"},
	}
	for _, c := range cases {
		if got := FormatDateDisplay(c.in); got != c.want {
			t.Errorf("FormatDateDisplay(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDateCSV(t *testing.T) {
	if got := formatDateCSV("20260906"); got != "2026-09-06" {
		t.Errorf("formatDateCSV full = %q", got)
	}
	// Short inputs fall back to the display format with placeholders.
	if got := formatDateCSV("2026"); !strings.Contains(got, "2026") {
		t.Errorf("formatDateCSV short = %q", got)
	}
}

func TestTodayDateDigits(t *testing.T) {
	d := todayDateDigits()
	if len(d) != 8 {
		t.Fatalf("todayDateDigits() = %q, want 8 digits", d)
	}
	for _, c := range d {
		if c < '0' || c > '9' {
			t.Fatalf("todayDateDigits() = %q, want only digits", d)
		}
	}
	if todayDate() != formatDateCSV(d) {
		t.Errorf("todayDate() = %q, digits render as %q", todayDate(), formatDateCSV(d))
	}
}
