package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// The cameras and microphones ffmpeg can see, for the settings page.

type avDevice struct {
	Index int
	Name  string
}

var devLine = regexp.MustCompile(`\[(\d+)\] (.+)$`)

// listCaptureDevices asks ffmpeg. On a Mac the answer is two numbered lists; the capture
// device setting is "<camera>:<microphone>" by index. Elsewhere the lists stay empty and
// the setting is edited as text.
func listCaptureDevices(ffmpeg string) (video, audio []avDevice) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}
	out, _ := exec.Command(ffmpeg, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "").CombinedOutput()
	section := ""
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.Contains(line, "video devices"):
			section = "video"
			continue
		case strings.Contains(line, "audio devices"):
			section = "audio"
			continue
		}
		m := devLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		d := avDevice{Index: idx, Name: strings.TrimSpace(m[2])}
		if strings.HasPrefix(d.Name, "Capture screen") {
			continue // screens are not cameras
		}
		switch section {
		case "video":
			video = append(video, d)
		case "audio":
			audio = append(audio, d)
		}
	}
	return video, audio
}

// splitDevice reads "<camera>:<microphone>" from the setting.
func splitDevice(setting string) (cam, mic int) {
	parts := strings.SplitN(setting, ":", 2)
	cam, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	if len(parts) == 2 {
		mic, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	return cam, mic
}

func joinDevice(cam, mic int) string { return fmt.Sprintf("%d:%d", cam, mic) }

func deviceName(list []avDevice, idx int) string {
	for _, d := range list {
		if d.Index == idx {
			return d.Name
		}
	}
	return fmt.Sprintf("device %d", idx)
}
