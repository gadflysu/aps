package filter

import (
	"testing"
	"time"
)

// --- ParseDateExpr tests ---

func TestParseDateExpr_YYYYMMDD(t *testing.T) {
	got, err := ParseDateExpr("2026-06-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_YYYYMMDDHHMM(t *testing.T) {
	got, err := ParseDateExpr("2026-06-01 14:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 6, 1, 14, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_Today(t *testing.T) {
	got, err := ParseDateExpr("today")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Now()
	want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_Yesterday(t *testing.T) {
	got, err := ParseDateExpr("yesterday")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Now()
	want := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_NDaysAgo(t *testing.T) {
	got, err := ParseDateExpr("3 days ago")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Now().AddDate(0, 0, -3)
	want = time.Date(want.Year(), want.Month(), want.Day(), 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_NWeeksAgo(t *testing.T) {
	got, err := ParseDateExpr("2 weeks ago")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Now().AddDate(0, 0, -14)
	want = time.Date(want.Year(), want.Month(), want.Day(), 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_NMonthsAgo(t *testing.T) {
	got, err := ParseDateExpr("1 month ago")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Now().AddDate(0, -1, 0)
	want = time.Date(want.Year(), want.Month(), want.Day(), 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_SingularUnits(t *testing.T) {
	// "1 day ago" should also work
	got, err := ParseDateExpr("1 day ago")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Now().AddDate(0, 0, -1)
	want = time.Date(want.Year(), want.Month(), want.Day(), 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDateExpr_InvalidFormat(t *testing.T) {
	cases := []string{
		"",
		"not-a-date",
		"2026-13-01",  // invalid month
		"2026-06-32",  // invalid day
		"abc days ago",
		"0 days ago",  // zero is ambiguous
	}
	for _, c := range cases {
		_, err := ParseDateExpr(c)
		if err == nil {
			t.Errorf("ParseDateExpr(%q) should return error", c)
		}
	}
}

func TestParseDateExpr_CaseInsensitive(t *testing.T) {
	cases := []string{"Today", "YESTERDAY", "Today"}
	for _, c := range cases {
		_, err := ParseDateExpr(c)
		if err != nil {
			t.Errorf("ParseDateExpr(%q) should not error: %v", c, err)
		}
	}
}

// --- DateInRange tests ---

func TestDateInRange_BothBounds(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	until := time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local)

	mid := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	if !DateInRange(mid, &from, &until) {
		t.Error("mid-range session should match")
	}

	before := time.Date(2026, 5, 31, 23, 59, 59, 0, time.Local)
	if DateInRange(before, &from, &until) {
		t.Error("before-range session should not match")
	}

	after := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if DateInRange(after, &from, &until) {
		t.Error("after-range session should not match")
	}
}

func TestDateInRange_InclusiveBounds(t *testing.T) {
	bound := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)

	// Exactly at from bound
	if !DateInRange(bound, &bound, nil) {
		t.Error("session exactly at --from should match (inclusive)")
	}

	// Exactly at until bound
	if !DateInRange(bound, nil, &bound) {
		t.Error("session exactly at --until should match (inclusive)")
	}
}

func TestDateInRange_OnlyFrom(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)

	after := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	if !DateInRange(after, &from, nil) {
		t.Error("session after --from should match when --until is nil")
	}

	before := time.Date(2026, 5, 15, 0, 0, 0, 0, time.Local)
	if DateInRange(before, &from, nil) {
		t.Error("session before --from should not match")
	}
}

func TestDateInRange_OnlyUntil(t *testing.T) {
	until := time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local)

	before := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	if !DateInRange(before, nil, &until) {
		t.Error("session before --until should match when --from is nil")
	}

	after := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	if DateInRange(after, nil, &until) {
		t.Error("session after --until should not match")
	}
}

func TestDateInRange_NoFilter(t *testing.T) {
	now := time.Now()
	if !DateInRange(now, nil, nil) {
		t.Error("no date filter should match everything")
	}
}

func TestDateInRange_ZeroTime(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	// Zero time (session with no timestamp) should not match a date filter
	if DateInRange(time.Time{}, &from, nil) {
		t.Error("zero-time session should not match date filter")
	}
	// Zero time with no filter should match
	if !DateInRange(time.Time{}, nil, nil) {
		t.Error("zero-time session should match when no date filter")
	}
}
