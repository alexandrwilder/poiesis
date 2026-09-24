package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Poiesis links are the one way anything outside the app asks it for something: the menu
// bar, a note, a shortcut, an AI. They are written down in docs/FORMAT.md, so anyone can make
// them:
//
//	poiesis://record?about=<what to talk about>   open ready to record; the person presses space
//	poiesis://<entry>?t=<seconds>                  open an entry at that moment
//
// A running window reads them from the command file (window.go); the system hands them to
// the Mac app, and `poiesis open <link>` works anywhere.

const linkScheme = "poiesis://"

// maxAbout keeps the line to talk about to one line on the record screen.
const maxAbout = 120

var entryIDPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-[a-z]+$`)

type link struct {
	record bool
	about  string  // with record: what the person wants to talk about
	entry  string  // an entry to open
	t      float64 // with entry: the second to open it at
}

// parseLink reads a link, or refuses it. The bare word "record" is what the menu bar item
// has always written.
func parseLink(s string) (link, error) {
	s = strings.TrimSpace(s)
	if s == "record" {
		return link{record: true}, nil
	}
	if !strings.HasPrefix(s, linkScheme) {
		return link{}, fmt.Errorf("not a Poiesis link: %q", s)
	}
	u, err := url.Parse(s)
	if err != nil {
		return link{}, fmt.Errorf("not a Poiesis link: %q", s)
	}
	switch {
	case u.Host == "record":
		about := strings.Join(strings.Fields(u.Query().Get("about")), " ") // one line
		if r := []rune(about); len(r) > maxAbout {
			about = strings.TrimSpace(string(r[:maxAbout]))
		}
		return link{record: true, about: about}, nil
	case entryIDPattern.MatchString(u.Host):
		t, _ := strconv.ParseFloat(u.Query().Get("t"), 64)
		return link{entry: u.Host, t: max0(t)}, nil
	}
	return link{}, fmt.Errorf("Poiesis has no link called %q", u.Host)
}

// recordLink is the link that opens Poiesis ready to record, with a line to talk about.
func recordLink(about string) string {
	if about = strings.TrimSpace(about); about == "" {
		return linkScheme + "record"
	}
	return linkScheme + "record?about=" + url.QueryEscape(about)
}
