package checktui

import "testing"

func TestLogicalBodiesDependOnlyOnSeed(t *testing.T) {
	a, b, c, d := logicalBodies("fixed-seed")
	a2, b2, c2, d2 := logicalBodies("fixed-seed")
	if a != a2 || b != b2 || c != c2 || d != d2 {
		t.Fatal("same seed changed logical messages")
	}
	other, _, _, _ := logicalBodies("different-seed")
	if a == other || a == b || b == c || c == d {
		t.Fatal("seed or message type not represented")
	}
}
