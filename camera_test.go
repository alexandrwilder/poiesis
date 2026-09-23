package main

import (
	"testing"
	"time"
)

// fakeCamera stands in for ffmpeg or a host, so the record screen can be tested without a
// camera. It is the core's side of the seam in docs/ARCHITECTURE.md.
type fakeCamera struct {
	lvl     float64
	el      time.Duration
	stopped bool
	end     chan error
}

func (f *fakeCamera) stop() error            { f.stopped = true; return nil }
func (f *fakeCamera) level() float64         { return f.lvl }
func (f *fakeCamera) elapsed() time.Duration { return f.el }
func (f *fakeCamera) picture() *reflection   { return nil }
func (f *fakeCamera) ended() <-chan error    { return f.end }

// A camera that fails to start must come back as no camera at all: a typed nil inside the
// interface would look like a running camera to every "cap != nil" check.
func TestOpenCameraFailureIsNoCamera(t *testing.T) {
	v := &Vault{Config: defaultConfig()}
	v.Config.FFmpegBin = "poiesis-test-no-such-ffmpeg"
	c, err := openCamera(v, captureOptions{PreviewW: 64, PreviewH: 36})
	if err == nil {
		t.Fatal("starting a camera without ffmpeg must fail")
	}
	if c != nil {
		t.Fatalf("a failed start gave a non-nil camera: %#v", c)
	}
}

// The record screen counts, pauses and stops through the seam only.
func TestRecordScreenUsesTheSeam(t *testing.T) {
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, focused: true}
	m.v.Config.Reflection = "off" // no picture: pausing must not open a real camera here
	f := &fakeCamera{lvl: 0.5, el: 3 * time.Second, end: make(chan error, 1)}
	m.record.cap = f
	m.record.phase = "recording"
	if got := m.totalRecorded(); got != 3*time.Second {
		t.Fatalf("recorded %v while the camera says 3s", got)
	}
	m.pauseEntry()
	if !f.stopped || m.record.cap != nil || m.record.phase != "paused" {
		t.Fatalf("pausing should stop the camera and keep the entry: stopped=%v cap=%v phase=%s", f.stopped, m.record.cap, m.record.phase)
	}
	if m.record.recorded != 3*time.Second {
		t.Fatalf("the paused entry kept %v, want 3s", m.record.recorded)
	}
}
