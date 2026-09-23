package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// hostCamera is the camera when a host draws the picture and records (docs/HOST.md). The
// frames never reach the core, so picture() is nil and the record screen draws only text.
type hostCamera struct {
	h       *hostLink
	record  bool
	mu      sync.Mutex
	started time.Time
	stopped chan hostRecording
	end     chan error
	quit    chan struct{}
}

func openHostCamera(h *hostLink, opts captureOptions) (*hostCamera, error) {
	c := &hostCamera{h: h, record: opts.Record, stopped: make(chan hostRecording, 1),
		end: make(chan error, 1), quit: make(chan struct{})}
	if err := h.send(pictureMsg(opts.PreviewW > 0, opts.PreviewFPS)); err != nil {
		return nil, err
	}
	if opts.Record {
		if err := h.send(map[string]any{"t": "record", "action": "start", "file": opts.Out}); err != nil {
			return nil, err
		}
	}
	go c.watch()
	return c, nil
}

// pictureMsg asks the host for the picture with this theme's look, or for it to rest.
func pictureMsg(on bool, fps int) map[string]any {
	if !on {
		return map[string]any{"t": "picture", "on": false}
	}
	return map[string]any{"t": "picture", "on": true, "look": currentLook.Name, "strength": currentStrength,
		"matrix": currentLook.matrix(currentStrength), "fps": fps}
}

func (c *hostCamera) watch() {
	for {
		select {
		case r := <-c.h.recording:
			switch r.state {
			case "started":
				c.mu.Lock()
				c.started = time.Now()
				c.mu.Unlock()
			case "stopped":
				select {
				case c.stopped <- r:
				default:
				}
			}
		case err := <-c.h.errs:
			select {
			case c.end <- err:
			default:
			}
		case <-c.h.closed:
			select {
			case c.end <- errors.New("the window closed its connection"):
			default:
			}
			return
		case <-c.quit:
			return
		}
	}
}

// stop ends this use of the camera: a recording is finished and written, then the picture
// rests. Like ffmpeg's stop, it waits at most fifteen seconds for the file.
func (c *hostCamera) stop() error {
	defer close(c.quit)
	var err error
	if c.record {
		if err = c.h.send(map[string]any{"t": "record", "action": "stop"}); err == nil {
			select {
			case <-c.stopped:
			case <-time.After(15 * time.Second):
				err = fmt.Errorf("the window did not finish the recording in time; the file may be incomplete")
			}
		}
	}
	_ = c.h.send(pictureMsg(false, 0))
	return err
}

func (c *hostCamera) level() float64       { return c.h.level() }
func (c *hostCamera) picture() *reflection { return nil }
func (c *hostCamera) ended() <-chan error  { return c.end }

func (c *hostCamera) elapsed() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.record || c.started.IsZero() {
		return 0
	}
	return time.Since(c.started)
}
