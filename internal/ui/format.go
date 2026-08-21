package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ANSI color codes
const (
	Reset               = "\033[0m"
	Bold                = "\033[1m"
	Red                 = "\033[31m"
	Green               = "\033[32m"
	Yellow              = "\033[33m"
	Blue                = "\033[34m"
	Cyan                = "\033[36m"
	CursorHome          = "\033[H"
	ClearFromCursorDown = "\033[J"
)

// StatusName returns a clean, human-readable name for an actor status enum.
func StatusName(status ateapipb.Actor_Status) string {
	str := status.String()
	if strings.HasPrefix(str, "STATUS_") {
		return str[7:]
	}
	return str
}

// FormatDuration formats a duration into a human-friendly compact string.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int64(d.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := int64(d.Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int64(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

// FormatAge returns the formatted age of a protobuf Timestamp relative to now.
func FormatAge(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "<unknown>"
	}
	t := ts.AsTime()
	if t.IsZero() {
		return "<unknown>"
	}
	return FormatDuration(time.Since(t))
}
