package filter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDateExpr parses a user-provided date expression into a time.Time.
//
// Supported formats:
//   - "2026-06-01"           (YYYY-MM-DD, midnight local)
//   - "2026-06-01 14:30"    (YYYY-MM-DD HH:MM, local)
//   - "today"               (midnight today, local)
//   - "yesterday"            (midnight yesterday, local)
//   - "N days ago"           (midnight, N days back)
//   - "N weeks ago"          (midnight, N*7 days back)
//   - "N months ago"         (midnight, N months back)
//
// The input is case-insensitive. Returns an error for unparseable input.
func ParseDateExpr(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date expression")
	}
	sLower := strings.ToLower(s)

	// 1. Relative keywords
	switch sLower {
	case "today":
		return startOfDay(time.Now()), nil
	case "yesterday":
		return startOfDay(time.Now().AddDate(0, 0, -1)), nil
	}

	// 2. "N units ago" pattern
	if m := agoRe.FindStringSubmatch(sLower); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("invalid count in %q", s)
		}
		unit := m[2]
		var t time.Time
		switch {
		case strings.HasPrefix(unit, "day"):
			t = time.Now().AddDate(0, 0, -n)
		case strings.HasPrefix(unit, "week"):
			t = time.Now().AddDate(0, 0, -n*7)
		case strings.HasPrefix(unit, "month"):
			t = time.Now().AddDate(0, -n, 0)
		default:
			return time.Time{}, fmt.Errorf("unknown unit %q in %q", unit, s)
		}
		return startOfDay(t), nil
	}

	// 3. Absolute date: YYYY-MM-DD HH:MM
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return t, nil
	}

	// 4. Absolute date: YYYY-MM-DD
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unrecognized date expression: %q", s)
}

// DateInRange reports whether sessionTime falls within the [from, until] range.
// A nil bound means unbounded on that side. A zero sessionTime never matches
// when any bound is set (sessions without timestamps are excluded).
func DateInRange(sessionTime time.Time, from, until *time.Time) bool {
	if from == nil && until == nil {
		return true
	}
	// Zero-time sessions have no meaningful timestamp; exclude them.
	if sessionTime.IsZero() {
		return false
	}
	if from != nil && sessionTime.Before(*from) {
		return false
	}
	if until != nil && sessionTime.After(*until) {
		return false
	}
	return true
}

// agoRe matches "N units ago" where N > 0 and unit starts with day/week/month.
var agoRe = regexp.MustCompile(`^(\d+)\s+(day|week|month)s?\s+ago$`)

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}
