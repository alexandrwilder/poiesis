package main

import "testing"

// The mark is drawn by hand; a row of the wrong width would shear it on screen.
func TestWordmarkRows(t *testing.T) {
	if len(wordmark) != 5 {
		t.Fatalf("the mark has %d rows, want 5", len(wordmark))
	}
	w := len([]rune(wordmark[0]))
	for i, r := range wordmark {
		if got := len([]rune(r)); got != w {
			t.Fatalf("row %d is %d cells wide, row 0 is %d", i, got, w)
		}
	}
}
