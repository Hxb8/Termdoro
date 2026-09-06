package app

import (
	"time"
)

// unixToDate converts Unix epoch seconds to a "YYYY-MM-DD" date string in UTC.
func unixToDate(secs uint64) string {
	return time.Unix(int64(secs), 0).UTC().Format("2006-01-02")
}

// todayDate returns today's date as "YYYY-MM-DD" in UTC.
func todayDate() string {
	return unixToDate(uint64(time.Now().Unix()))
}

// todayDateDigits returns today's date as "YYYYMMDD".
func todayDateDigits() string {
	t := time.Now().UTC()
	return t.Format("20060102")
}

// FormatDateDisplay renders up to 8 "YYYYMMDD" digits with dashes
// auto-inserted and missing digits shown as underscores, e.g. "2026-09-__".
func FormatDateDisplay(digits string) string {
	var out []byte
	for i := 0; i < 8; i++ {
		if i == 4 || i == 6 {
			out = append(out, '-')
		}
		if i < len(digits) {
			out = append(out, digits[i])
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// formatDateCSV converts "YYYYMMDD" digits to "YYYY-MM-DD" for CSV storage.
// Short inputs fall back to the display format.
func formatDateCSV(digits string) string {
	if len(digits) == 8 {
		return digits[0:4] + "-" + digits[4:6] + "-" + digits[6:8]
	}
	return FormatDateDisplay(digits)
}
