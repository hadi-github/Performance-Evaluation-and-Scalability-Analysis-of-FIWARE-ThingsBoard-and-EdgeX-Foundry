package hash

import (
	"math"
	"testing"
)

func TestDeterministicAndDistinct(t *testing.T) {
	if Mix(1, 2, SaltSelect) != Mix(1, 2, SaltSelect) {
		t.Fatal("not deterministic")
	}
	seen := map[uint32]bool{}
	for seq := uint32(0); seq < 100000; seq++ {
		h := Mix(42, seq, SaltSelect)
		if seen[h] {
			t.Fatalf("collision at seq %d: Mix must be a bijection on seq", seq)
		}
		seen[h] = true
	}
}

func TestThinningRate(t *testing.T) {
	for _, p := range []float64{0.5, 0.1, 0.05} {
		th, n, N := Threshold(p), 0, 200000
		for seq := 0; seq < N; seq++ {
			if Selected(7, uint32(seq), th) {
				n++
			}
		}
		got, sd := float64(n)/float64(N), math.Sqrt(p*(1-p)/float64(N))
		if math.Abs(got-p) > 5*sd {
			t.Fatalf("p=%v: selected fraction %v (more than 5 sd off)", p, got)
		}
	}
}

func TestValueRanges(t *testing.T) {
	for seq := uint32(0); seq < 10000; seq++ {
		tt, h, p := Values(3, seq)
		if tt < TemperatureMin || tt > TemperatureMax || h < HumidityMin || h > HumidityMax ||
			p < PressureMin || p > PressureMax {
			t.Fatalf("seq %d out of range: %d %d %d", seq, tt, h, p)
		}
	}
}
