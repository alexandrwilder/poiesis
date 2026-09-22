package main

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
)

// Processing is not a black box: whisper reports its progress in percent when asked, and
// extraction runs window by window. Both write here; the record screen reads it.

type progressState struct {
	mu    sync.Mutex
	stage string // "transcribing" | "extracting" | ""
	pct   int    // transcribing: 0..100
	done  int    // extracting: windows done
	total int    // extracting: windows in all
}

var progress progressState

func setProgress(stage string, pct, done, total int) {
	progress.mu.Lock()
	progress.stage, progress.pct, progress.done, progress.total = stage, pct, done, total
	progress.mu.Unlock()
}

func readProgress() (stage string, pct, done, total int) {
	progress.mu.Lock()
	defer progress.mu.Unlock()
	return progress.stage, progress.pct, progress.done, progress.total
}

var pctRe = regexp.MustCompile(`progress\s*=\s*(\d+)%`)

// runReporting runs a command, keeps everything it prints, and feeds any "progress = N%"
// line into the shared progress as it happens.
func runReporting(stage string, cmd *exec.Cmd) ([]byte, error) {
	var out bytes.Buffer
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanDone := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			line := sc.Bytes()
			out.Write(line)
			out.WriteByte('\n')
			if m := pctRe.FindSubmatch(line); m != nil {
				if n, err := strconv.Atoi(string(m[1])); err == nil {
					setProgress(stage, n, 0, 0)
				}
			}
		}
		close(scanDone)
	}()
	err := cmd.Wait()
	pw.Close()
	<-scanDone
	return out.Bytes(), err
}
