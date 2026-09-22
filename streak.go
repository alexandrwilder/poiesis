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
		if days[i].Sub(days[i-1]) == 24*time.Hour {
			run++
		} else {
			break
		}
	}
	last := days[len(days)-1]
	t0 := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	alive := !last.Before(t0.Add(-24 * time.Hour))
	if !alive {
		run = 0
	}
	return streak{Days: len(days), Run: run, Alive: alive}
}
