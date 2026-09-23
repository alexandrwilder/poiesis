package main

import (
	"math"
	"math/rand"
	"testing"
)

// A host shows the look through its colour matrix, so the matrix must give the same pixels as
// the look itself: every look, three strengths, pixels across the whole range, within one
// level of 255.
func TestLookMatrixMatchesTheLook(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, l := range looks {
		for _, k := range []float64{0.3, 0.7, 1} {
			m := l.matrix(k)
			for i := 0; i < 500; i++ {
				r, g, b := uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))
				want := l.pixel(k, r, g, b)
				in := [3]float64{float64(r) / 255, float64(g) / 255, float64(b) / 255}
				for row := 0; row < 3; row++ {
					v := m[row*4]*in[0] + m[row*4+1]*in[1] + m[row*4+2]*in[2] + m[row*4+3]
					got := math.Max(0, math.Min(255, v*255))
					if math.Abs(got-float64(want[row])) > 1 {
						t.Fatalf("%s at %.1f, pixel %d %d %d, channel %d: matrix gives %.1f, the look %d", l.Name, k, r, g, b, row, got, want[row])
					}
				}
			}
		}
	}
}
