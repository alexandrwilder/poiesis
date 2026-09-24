package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Days are counted by the calendar: a clock change never adds or loses a day. The next
// change is 25 October; a day number written wrong stays wrong in the entry page for good.
func TestDaysAcrossTheClockChanges(t *testing.T) {
	sthlm, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	at := func(s string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", s, sthlm)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"2026-10-24 12:00", "2026-10-26 12:00", 2}, // the autumn change, a 25-hour day
		{"2026-03-28 12:00", "2026-03-30 12:00", 2}, // the spring change, a 23-hour day
		{"2026-03-29 23:30", "2026-03-30 00:10", 1},
		{"2026-09-24 00:01", "2026-09-24 23:59", 0},
	} {
		if got := calendarDays(at(c.a), at(c.b)); got != c.want {
			t.Errorf("%s to %s: %d days, want %d", c.a, c.b, got, c.want)
		}
	}

	// three days in a row across each change are a run of three
	for _, days := range [][]string{
		{"2026-10-24 20:00", "2026-10-25 20:00", "2026-10-26 20:00"},
		{"2026-03-28 20:00", "2026-03-29 20:00", "2026-03-30 20:00"},
	} {
		var rec []string
		for _, d := range days {
			rec = append(rec, at(d).Format(time.RFC3339))
		}
		if s := daysLogged(rec, at(days[2])); s.Run != 3 || !s.Alive {
			t.Errorf("%v: run %d, alive %v; want 3, true", days, s.Run, s.Alive)
		}
	}

	// 15 April is day 105 when 1 January is day 1, whatever the clocks did in March
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "episodes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "episodes", "2026-01-01-a.md"), []byte("---\nid: 2026-01-01-a\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := &Vault{Root: root}
	if day, err := v.Day(at("2026-04-15 09:00")); err != nil || day != 105 {
		t.Fatalf("day %d, %v; want 105", day, err)
	}
}
