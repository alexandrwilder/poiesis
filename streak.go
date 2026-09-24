package main

import (
	"sort"
	"time"
)

// The streak, like a Whoop day count: how many days carry at least one entry, and the
// current run of days in a row. A run is alive if its last day is today or yesterday;
// today is still open, so a run does not break until a whole day passes without an entry.

type streak struct {
	Days  int  // distinct days with an entry
	Run   int  // days in a row, ending today or yesterday
	Alive bool // false when the run ended before yesterday
}

// calendarDays counts the days from a's date to b's date, each in its own time zone. It
// counts by the calendar, so a clock change (a 23- or 25-hour day) never adds or loses one.
func calendarDays(a, b time.Time) int {
	ya, ma, da := a.Date()
	yb, mb, db := b.Date()
	return int(time.Date(yb, mb, db, 0, 0, 0, 0, time.UTC).Sub(time.Date(ya, ma, da, 0, 0, 0, 0, time.UTC)).Hours() / 24)
}

// daysLogged takes entry times (RFC3339) and "today" and returns the streak.
func daysLogged(recordedAt []string, today time.Time) streak {
	seen := map[string]bool{}
	var days []time.Time
	for _, s := range recordedAt {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			continue
		}
		t = t.In(today.Location())
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())
		if !seen[d.Format("2006-01-02")] {
			seen[d.Format("2006-01-02")] = true
			days = append(days, d)
		}
	}
	if len(days) == 0 {
		return streak{}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	run := 1
	for i := len(days) - 1; i > 0; i-- {
		if calendarDays(days[i-1], days[i]) == 1 {
			run++
		} else {
			break
		}
	}
	last := days[len(days)-1]
	alive := calendarDays(last, today) <= 1
	if !alive {
		run = 0
	}
	return streak{Days: len(days), Run: run, Alive: alive}
}
