package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// The core's side of docs/HOST.md: one connection to the host, made once at start.

const hostProtocol = 1

// theHost is the host this core runs in, or nil when it runs on its own.
var theHost *hostLink

type hostLink struct {
	conn      net.Conn
	name      string
	can       map[string]bool // written once while meeting, only read afterwards
	mu        sync.Mutex      // guards writes and levelDB
	levelDB   float64
	recording chan hostRecording
	errs      chan error
	closed    chan struct{}
}

type hostRecording struct {
	state   string // started | stopped
	file    string
	seconds float64
}

// hostMsg is every message a host may send; fields a type does not use stay empty.
type hostMsg struct {
	T       string   `json:"t"`
	Version int      `json:"version"`
	Host    string   `json:"host"`
	Can     []string `json:"can"`
	DB      *float64 `json:"db"`
	State   string   `json:"state"`
	File    string   `json:"file"`
	Seconds float64  `json:"seconds"`
	What    string   `json:"what"`
	Text    string   `json:"text"`
}

// connectHost meets the host named in POIESIS_HOST. No host, a host that does not answer
// within two seconds, or no shared version all give nil: the core then runs on its own.
func connectHost() *hostLink {
	path := os.Getenv("POIESIS_HOST")
	if path == "" {
		return nil
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil
	}
	h := &hostLink{conn: conn, can: map[string]bool{}, levelDB: -60,
		recording: make(chan hostRecording, 4), errs: make(chan error, 4), closed: make(chan struct{})}
	if err := h.send(map[string]any{"t": "hello", "versions": []int{hostProtocol}, "app": "poiesis " + version}); err != nil {
		conn.Close()
		return nil
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	var hello hostMsg
	if err != nil || json.Unmarshal(line, &hello) != nil || hello.T != "hello" || hello.Version != hostProtocol {
		conn.Close()
		return nil
	}
	_ = conn.SetReadDeadline(time.Time{})
	for _, c := range hello.Can {
		h.can[c] = true
	}
	h.name = hello.Host
	go h.read(r)
	return h
}

// read takes the host's messages until the connection closes. Unknown types and fields are
// ignored, as the contract says.
func (h *hostLink) read(r *bufio.Reader) {
	defer close(h.closed)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var m hostMsg
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		switch m.T {
		case "level":
			if m.DB != nil {
				h.mu.Lock()
				h.levelDB = *m.DB
				h.mu.Unlock()
			}
		case "recording":
			select {
			case h.recording <- hostRecording{state: m.State, file: m.File, seconds: m.Seconds}:
			default:
			}
		case "error":
			select {
			case h.errs <- fmt.Errorf("%s: %s", m.What, m.Text):
			default:
			}
		}
	}
}

func (h *hostLink) send(msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = h.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = h.conn.Write(append(b, '\n'))
	return err
}

// has says whether the host said it can do this when they met.
func (h *hostLink) has(what string) bool { return h.can[what] }

func (h *hostLink) level() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return meterFromDB(h.levelDB)
}
