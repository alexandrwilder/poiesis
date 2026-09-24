package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Ask: questions a person can put to their own log, grouped the way the log types in the
// research are grouped, answered by the local AI inside the app over the person's own
// claims and words. The same question can be copied for any other AI app.

type askSet struct {
	Name      string
	Questions []string
}

var askSets = []askSet{
	{"this week", []string{
		"What did I say I would do this week, and did I?",
		"What happened this week, in three lines?",
		"What did I decide this week?",
	}},
	{"building", []string{
		"What stopped me most often, and what did I do about it?",
		"What did I finish, and what is half done?",
		"Which smallest next step did I name and not take?",
	}},
	{"experiments", []string{
		"What did I expect, and what actually happened?",
		"Which belief of mine did the facts contradict?",
		"What did I learn that I would tell my past self?",
	}},
	{"direction", []string{
		"What am I betting on right now, and what does it cost?",
		"Which risks did I say I accept?",
		"Where did my direction change, and why?",
	}},
	{"inside", []string{
		"How have I been feeling, day by day?",
		"What am I pretending not to know?",
		"What do I keep circling back to?",
	}},
}

// askContext gathers what the model gets to read: the claims that match the question's
// words, the last week's claims, and the self page's states. Capped, newest first.
func askContext(d *tuiData, question string) string {
	seen := map[string]bool{}
	var picked []Claim
	add := func(cs []Claim) {
		for _, c := range cs {
			if !seen[c.ID] {
				seen[c.ID] = true
				picked = append(picked, c)
			}
		}
	}
	for _, w := range strings.Fields(strings.ToLower(question)) {
		w = strings.Trim(w, "?,.!\"'")
		if len(w) < 4 || askStop[w] {
			continue
		}
		add(d.searchClaims(w))
	}
	week := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	var recent []Claim
	for _, c := range d.claims {
		if c.StatedAt >= week {
			recent = append(recent, c)
		}
	}
	add(recent)
	add(d.claimsAbout("self"))
	sort.Slice(picked, func(i, j int) bool { return picked[i].StatedAt > picked[j].StatedAt })
	if len(picked) > 80 {
		picked = picked[:80]
	}
	var b strings.Builder
	for _, c := range picked {
		fmt.Fprintf(&b, "%s · %s · %s [%s @%.1f]\n", leading(c.StatedAt, 10), c.Kind, c.Text, c.Source.Episode, c.Source.Start)
	}
	return b.String()
}

var askStop = map[string]bool{"what": true, "which": true, "where": true, "when": true, "this": true, "that": true,
	"with": true, "have": true, "been": true, "most": true, "often": true, "about": true, "would": true, "could": true,
	"did": true, "and": true, "the": true, "week": true, "lines": true, "three": true, "right": true, "now": true,
	"keep": true, "back": true, "say": true, "said": true, "tell": true, "past": true, "self": true, "actually": true}

// askLocal puts the question and the context to the local AI and returns its answer.
func askLocal(v *Vault, question, context string) (string, error) {
	model := v.Config.Model
	if model == "" {
		model = setupOllamaModel
	}
	if err := ensureOllama(v, model); err != nil {
		return "", err
	}
	if strings.TrimSpace(context) == "" {
		return "The log has nothing on that yet. Say it to the camera, and ask again tomorrow.", nil
	}
	system := "You are reading one person's own video log. Below are their claims: date, kind, what they said in plain words, and the entry with the second it was said. " +
		"Answer their question in at most six short lines, in plain English, speaking to them as 'you'. Use only the log. " +
		"After each fact, cite it like [2026-09-03-a @12.5]. If the log does not say, say so in one line. No headings, no lists longer than six items."
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"stream": false,
		"think":  false,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": "The log:\n" + context + "\nThe question: " + question},
		},
		"options": map[string]any{"temperature": 0.2, "num_ctx": 16384},
	})
	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Post(strings.TrimRight(v.Config.OllamaURL, "/")+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("%s", out.Error)
	}
	return strings.TrimSpace(out.Message.Content), nil
}

// copyText puts text on the clipboard, the way each system does it.
func copyText(s string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
	}
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// forYourAI is the question with the instruction any AI app needs first.
func forYourAI(question string) string {
	return "Orient yourself in my Poiesis vault first. Then: " + question + " Cite the entry and the second for every fact, and give me the moment to play."
}
