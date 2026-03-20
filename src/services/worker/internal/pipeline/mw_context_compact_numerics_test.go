package pipeline

import "testing"

func TestPersistEffectiveHistMax(t *testing.T) {
	max := func(histTok, prevRunInput int) int {
		if prevRunInput > histTok {
			return prevRunInput
		}
		return histTok
	}
	if max(1000, 50_000) != 50_000 {
		t.Fatal("prev wins")
	}
	if max(80_000, 10_000) != 80_000 {
		t.Fatal("hist wins")
	}
}
