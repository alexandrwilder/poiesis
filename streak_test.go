package main

import (
	"testing"
	"time"
)

func TestDaysLogged(t *testing.T) {
	today := time.Date(2026, 9, 10, 20, 0, 0, 0, time.Local)
	at := func(d string) string { return d + "T08:00:00+02:00" }
	cases := []struct {
		name string
		in   []string
		days int
		run  int
	}{
		{"empty", nil, 0, 0},
		{"two entries one day", []string{at("2026-09-10"), at("2026-09-10")}, 1, 1},
		{"three days in a row incl today", []string{at("2026-09-08"), at("2026-09-09"), at("2026-09-10")}, 3, 3},
		{"run alive from yesterday", []string{at("2026-09-08"), at("2026-09-09")}, 2, 2},
		{"gap breaks the run", []string{at("2026-09-01"), at("2026-09-02"), at("2026-09-09"), at("2026-09-10")}, 4, 2},
		{"run ended before yesterday", []string{at("2026-09-05"), at("2026-09-06")}, 2, 0},
	}
	for _, c := range cases {
		s := daysLogged(c.in, today)
		if s.Days != c.days || s.Run != c.run {
			t.Errorf("%s: got days=%d run=%d, want days=%d run=%d", c.name, s.Days, s.Run, c.days, c.run)
		}
	}
}
