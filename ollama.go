package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// The local AI, part of the install: Poiesis carries the Ollama runtime inside the app and
// starts it when an entry needs extracting. The model itself (2.5 GB) is fetched once, on
// first use, into the app's own folder, so an update of Poiesis never fetches it again.
// If a person already runs their own Ollama, that one is used and nothing is started.

var ollamaChild *exec.Cmd

// ensureOllama makes sure a server answers and the model is there, reporting progress.
func ensureOllama(v *Vault, model string) error {
	if ollamaUp(v.Config.OllamaURL) {
		return ensureOllamaModel(v.Config.OllamaURL, model)
	}
	bin := findTool("ollama")
	if !strings.Contains(bin, "/") {
		return fmt.Errorf("no local AI runtime: install Ollama from ollama.com, or use your own key in settings")
	}
	dir := filepath.Join(appStateDir(), "ollama")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	c := exec.Command(bin, "serve")
	c.Env = append(os.Environ(), "OLLAMA_MODELS="+dir, "OLLAMA_HOST=127.0.0.1:11434")
	logf, _ := os.OpenFile(filepath.Join(appStateDir(), "ollama.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	c.Stdout, c.Stderr = logf, logf
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	setProgress("starting the local AI", 0, 0, 0)
	if err := c.Start(); err != nil {
		return fmt.Errorf("could not start the local AI: %w", err)
	}
	ollamaChild = c
	for i := 0; i < 100; i++ { // up to ten seconds
		if ollamaUp(v.Config.OllamaURL) {
			return ensureOllamaModel(v.Config.OllamaURL, model)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the local AI did not answer within ten seconds (see ollama.log in the app's folder)")
}

// stopOllama ends a runtime Poiesis started itself. Someone else's is left alone.
func stopOllama() {
	if ollamaChild != nil && ollamaChild.Process != nil {
		_ = ollamaChild.Process.Kill()
		ollamaChild = nil
	}
}

func ollamaUp(url string) bool {
	client := &http.Client{Timeout: 700 * time.Millisecond}
	resp, err := client.Get(strings.TrimRight(url, "/") + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// ensureOllamaModel pulls the model if it is not there, with progress in percent.
func ensureOllamaModel(url, model string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(url, "/") + "/api/tags")
	if err == nil {
		var tags struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&tags)
		resp.Body.Close()
		for _, m := range tags.Models {
			if m.Name == model {
				return nil
			}
		}
	}
	setProgress("fetching the local AI, once", 0, 0, 0)
	body, _ := json.Marshal(map[string]any{"name": model, "stream": true})
	long := &http.Client{Timeout: 0}
	resp, err = long.Post(strings.TrimRight(url, "/")+"/api/pull", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("fetching the local AI: %w", err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var st struct {
			Status    string `json:"status"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &st) != nil {
			continue
		}
		if st.Error != "" {
			return fmt.Errorf("fetching the local AI: %s", st.Error)
		}
		if st.Total > 0 {
			setProgress("fetching the local AI, once", int(st.Completed*100/st.Total), 0, 0)
		}
	}
	return sc.Err()
}
