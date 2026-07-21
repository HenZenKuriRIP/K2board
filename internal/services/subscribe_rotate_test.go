package services

import (
	"testing"

	"K2board/internal/models"
)

func TestRotateOffsetUniform(t *testing.T) {
	n := 5
	counts := make([]int, n)
	// Fixed user salt; vary seq → each position hit equally often
	const rounds = 500
	for seq := int64(1); seq <= rounds; seq++ {
		off := RotateOffsetForTest(seq, 42, n)
		if off < 0 || off >= n {
			t.Fatalf("offset out of range: %d", off)
		}
		counts[off]++
	}
	// each offset should appear rounds/n times exactly (seq-1 + const) % n walks all residues
	want := rounds / n
	for i, c := range counts {
		if c != want {
			t.Fatalf("offset %d count=%d want %d (counts=%v)", i, c, want, counts)
		}
	}
}

func TestRotateNodesFairPreservesSet(t *testing.T) {
	nodes := []models.Node{
		{ID: 10, Name: "a"},
		{ID: 20, Name: "b"},
		{ID: 30, Name: "c"},
	}
	u := &models.User{ID: 7, PlanID: 3, GroupID: 1}
	out := RotateNodesFair(u, nodes)
	if len(out) != 3 {
		t.Fatalf("len %d", len(out))
	}
	seen := map[uint]bool{}
	for _, n := range out {
		seen[n.ID] = true
	}
	for _, id := range []uint{10, 20, 30} {
		if !seen[id] {
			t.Fatalf("missing id %d", id)
		}
	}
	// original slice not mutated
	if nodes[0].ID != 10 {
		t.Fatal("input mutated")
	}
}

func TestRotateNodesFairSingle(t *testing.T) {
	nodes := []models.Node{{ID: 1, Name: "only"}}
	out := RotateNodesFair(&models.User{ID: 1, PlanID: 1}, nodes)
	if len(out) != 1 || out[0].ID != 1 {
		t.Fatal(out)
	}
}

func TestSubscribeRotateKey(t *testing.T) {
	if subscribeRotateKey(&models.User{PlanID: 9, GroupID: 2}) != "k2board:subrot:plan:9" {
		t.Fatal(subscribeRotateKey(&models.User{PlanID: 9, GroupID: 2}))
	}
	if subscribeRotateKey(&models.User{PlanID: 0, GroupID: 4}) != "k2board:subrot:group:4" {
		t.Fatal(subscribeRotateKey(&models.User{GroupID: 4}))
	}
}
