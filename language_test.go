package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseDetected(t *testing.T) {
	g, ok := parseDetected("whisper_full_with_state: auto-detected language: sv (p = 0.996639)\n")
	if !ok || g.Lang != "sv" || g.P != 0.996639 {
		t.Fatalf("got %+v %v", g, ok)
	}
	if _, ok := parseDetected("read_audio_data: trying to decode with miniaudio"); ok {
		t.Fatal("a line without a detection must give no guess")
	}
}

// The rule the app lives by: switch to the Swedish model only when every stretch agrees.
func TestClearLanguage(t *testing.T) {
	cases := []struct {
		name string
		in   []langGuess
		want string
	}{
		{"Swedish throughout", []langGuess{{"sv", 0.997}, {"sv", 0.997}, {"sv", 0.996}}, "sv"},
		// measured on the first real entry: it opened in Swedish and went on in English
		{"a mix stays with the general model", []langGuess{{"sv", 0.992}, {"en", 0.999}, {"en", 0.994}}, ""},
		{"one unsure stretch is not clear", []langGuess{{"sv", 0.99}, {"sv", 0.6}}, ""},
		{"nothing heard", nil, ""},
		{"English throughout", []langGuess{{"en", 0.99}, {"en", 0.98}}, "en"},
	}
	for _, c := range cases {
		if got := clearLanguage(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestListenAt(t *testing.T) {
	cases := map[float64][]float64{20: {0}, 60: {0, 30}, 180: {0, 75, 150}}
	for dur, want := range cases {
		if got := listenAt(dur); !reflect.DeepEqual(got, want) {
			t.Errorf("listenAt(%v) = %v, want %v", dur, got, want)
		}
	}
}

// With real recordings and the models on this machine:
// POIESIS_ROUTE_WAVS="/path/a.wav=sv,/path/b.wav=" go test -run TestRouteOnRealAudio
// An empty expectation means the general model should read it.
func TestRouteOnRealAudio(t *testing.T) {
	spec := os.Getenv("POIESIS_ROUTE_WAVS")
	if spec == "" {
		t.Skip("set POIESIS_ROUTE_WAVS to wav=expected pairs to run on real audio")
	}
	v := &Vault{Config: defaultConfig()}
	dirs := []string{v.Config.ModelsDir, defaultModelsDir()}
	for _, item := range strings.Split(spec, ",") {
		wav, want, _ := strings.Cut(item, "=")
		st, err := os.Stat(wav)
		if err != nil {
			t.Fatal(err)
		}
		dur := float64(st.Size()-44) / 32000 // 16 kHz mono 16-bit
		if got := routeLanguage(v, dirs, wav, dur); got != want {
			t.Errorf("%s: got %q, want %q", wav, got, want)
		}
	}
}
