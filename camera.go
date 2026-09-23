package main

import "time"

// camera is what the record screen uses for the picture, the recording and the meter.
// Behind it sits either ffmpeg run by the core (capture.go) or a host over HOST.md; the
// record screen knows only this. See docs/ARCHITECTURE.md.
type camera interface {
	stop() error
	level() float64         // the microphone, 0 to 1, for the meter
	elapsed() time.Duration // recorded so far; zero until the recording has started
	picture() *reflection   // frames for the core to draw, or nil when a host draws them
	ended() <-chan error    // receives when the camera stops on its own
}

// openCamera starts the camera on the best path this machine has: the host when there is
// one that can do what is asked, otherwise ffmpeg run by the core.
func openCamera(v *Vault, opts captureOptions) (camera, error) {
	if h := theHost; h != nil && h.has("picture") && (!opts.Record || h.has("record")) {
		if c, err := openHostCamera(h, opts); err == nil {
			return c, nil
		}
		// the host did not take it: ffmpeg below
	}
	c, err := startCapture(v, opts)
	if err != nil {
		return nil, err // a nil *capture must never become a non-nil camera
	}
	return c, nil
}
