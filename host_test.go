package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHost plays the host's side of docs/HOST.md over a real socket: it answers hello with
// the given version, says what it can do, sends a meter level and a message type from some
// future version, and answers record start and stop. Every message the core sends lands in got.
func fakeHost(t *testing.T, version int) (sock string, got chan map[string]any) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ph") // unix socket paths must stay short
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock = filepath.Join(dir, "h.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan map[string]any, 32)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		say := func(s string) { conn.Write([]byte(s + "\n")) }
		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(line, &m) != nil {
				continue
			}
			got <- m
			switch m["t"] {
			case "hello":
				if version != 1 {
					say(`{"t":"hello","version":0}`)
					continue
				}
				say(`{"t":"hello","version":1,"host":"fake","can":["picture","look","record","level"]}`)
				say(`{"t":"from-a-future-version","x":1}`)
				say(`{"t":"level","db":-35}`)
			case "record":
				if m["action"] == "start" {
					say(`{"t":"recording","state":"started"}`)
				} else {
					say(`{"t":"recording","state":"stopped","file":"part01.mp4","seconds":1.5}`)
				}
			}
		}
	}()
	return sock, got
}

func next(t *testing.T, got chan map[string]any) map[string]any {
	t.Helper()
	select {
	case m := <-got:
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("the core sent nothing")
		return nil
	}
}

func TestNoHostWithoutTheVariable(t *testing.T) {
	t.Setenv("POIESIS_HOST", "")
	if connectHost() != nil {
		t.Fatal("without POIESIS_HOST the core must run on its own")
	}
}

func TestHostWithoutASharedVersionIsNotUsed(t *testing.T) {
	sock, _ := fakeHost(t, 0)
	t.Setenv("POIESIS_HOST", sock)
	if connectHost() != nil {
		t.Fatal("a host that shares no version must not be used")
	}
}

// The core's side of the contract, message by message.
func TestHostContractCoreSide(t *testing.T) {
	sock, got := fakeHost(t, 1)
	t.Setenv("POIESIS_HOST", sock)
	h := connectHost()
	if h == nil {
		t.Fatal("the core did not meet the host")
	}
	defer h.conn.Close()
	hello := next(t, got)
	if hello["t"] != "hello" || !strings.HasPrefix(hello["app"].(string), "poiesis ") {
		t.Fatalf("first message %v, want the core's hello", hello)
	}
	if !h.has("record") || h.has("teleport") {
		t.Fatal("the host's abilities were not taken as it said them")
	}
	theHost = h
	defer func() { theHost = nil }()
	v := &Vault{Config: defaultConfig()}

	// the picture: on with the theme's look, off again when the camera stops
	c, err := openCamera(v, captureOptions{PreviewW: 640, PreviewH: 360, PreviewFPS: 15})
	if err != nil || c.picture() != nil {
		t.Fatalf("with a host the camera must be the host's and draw nothing itself (err %v)", err)
	}
	if m := next(t, got); m["t"] != "picture" || m["on"] != true || m["look"] != currentLook.Name || m["fps"] != 15.0 {
		t.Fatalf("got %v, want the picture on with the look and 15 fps", m)
	}
	deadline := time.Now().Add(2 * time.Second)
	for c.level() != meterFromDB(-35) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if c.level() != meterFromDB(-35) {
		t.Fatalf("the meter shows %.2f, the host said -35 dB", c.level())
	}
	if err := c.stop(); err != nil {
		t.Fatal(err)
	}
	if m := next(t, got); m["t"] != "picture" || m["on"] != false {
		t.Fatalf("got %v, want the picture to rest", m)
	}

	// a recording: the picture, then start with the file; the clock runs once the host started
	rc, err := openCamera(v, captureOptions{Record: true, Out: "/tmp/part01.mp4", PreviewW: 640, PreviewH: 360, PreviewFPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	if m := next(t, got); m["t"] != "picture" || m["on"] != true {
		t.Fatalf("got %v, want the picture on while recording", m)
	}
	if m := next(t, got); m["t"] != "record" || m["action"] != "start" || m["file"] != "/tmp/part01.mp4" {
		t.Fatalf("got %v, want record start with the file", m)
	}
	deadline = time.Now().Add(2 * time.Second)
	for rc.elapsed() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rc.elapsed() == 0 {
		t.Fatal("the clock did not start when the host said started")
	}
	start := time.Now()
	if err := rc.stop(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("stop waited although the host said stopped")
	}
	if m := next(t, got); m["t"] != "record" || m["action"] != "stop" {
		t.Fatalf("got %v, want record stop", m)
	}
	if m := next(t, got); m["t"] != "picture" || m["on"] != false {
		t.Fatalf("got %v, want the picture to rest after the recording", m)
	}
}

// When a host draws the picture, the record screen draws the frame and text only.
func TestRecordScreenUnderAHostDrawsNoPicture(t *testing.T) {
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}, focused: true, width: 120, height: 40}
	m.v.Config.Reflection = "on"
	m.record.phase = "ready"
	m.record.cap = &fakeCamera{end: make(chan error, 1)}
	out := m.viewRecord()
	if strings.Contains(out, "\x1b_G") || strings.Contains(out, "█") || !m.fullWindow {
		t.Fatal("under a host the core must draw neither an image nor the mark, and must fill the window with its frame")
	}
}
